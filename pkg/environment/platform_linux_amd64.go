package environment

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func supported() error { return nil }

func openRegular(p string) (*os.File, error) {
	fd, err := syscall.Open(p, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), p)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("not a readable regular file: %s", p)
	}
	return f, nil
}

func lockDirectory(p string) (*os.File, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("environment is busy: %w", err)
	}
	return f, nil
}

func ownership(info os.FileInfo) (uint32, uint32) {
	s := info.Sys().(*syscall.Stat_t)
	return s.Uid, s.Gid
}

// Use RENAME_NOREPLACE, not a check-then-rename sequence that could overwrite
// a concurrently created destination. This adapter is deliberately linux/amd64.
func publishDirectory(from, to string) error {
	f, err := syscall.BytePtrFromString(from)
	if err != nil {
		return err
	}
	t, err := syscall.BytePtrFromString(to)
	if err != nil {
		return err
	}
	const sysRenameat2 = 316
	atFDCWD := -100
	_, _, errno := syscall.Syscall6(sysRenameat2, uintptr(atFDCWD), uintptr(unsafe.Pointer(f)), uintptr(atFDCWD), uintptr(unsafe.Pointer(t)), 1, 0)
	if errno != 0 {
		return fmt.Errorf("publish without overwrite: %w", errno)
	}
	return nil
}

func applyMetadata(p string, e Entry) error {
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	u, g := ownership(info)
	if u != e.UID || g != e.GID {
		if err := os.Lchown(p, int(e.UID), int(e.GID)); err != nil {
			return fmt.Errorf("restore ownership of %s: %w", e.Path, err)
		}
	}
	if e.Type == "symlink" {
		// Linux utimensat with AT_SYMLINK_NOFOLLOW preserves link timestamps
		// without following a possibly dangling target.
		name, err := syscall.BytePtrFromString(p)
		if err != nil {
			return err
		}
		times := [2]syscall.Timespec{{Sec: e.Mtime}, {Sec: e.Mtime}}
		atFDCWD := -100
		_, _, errno := syscall.Syscall6(syscall.SYS_UTIMENSAT, uintptr(atFDCWD), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&times[0])), 0x100, 0, 0)
		if errno != 0 {
			return errno
		}
		return nil
	}
	mode := os.FileMode(e.Mode & 0777)
	if e.Mode&01000 != 0 {
		mode |= os.ModeSticky
	}
	if err := os.Chmod(p, mode); err != nil {
		return err
	}
	t := time.Unix(e.Mtime, 0)
	return os.Chtimes(p, t, t)
}

func checkAttributes(p string, info os.FileInfo) error {
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return fmt.Errorf("setuid/setgid unsupported: %s", p)
	}
	// llistxattr avoids dereferencing symlinks, including dangling links.
	name, err := syscall.BytePtrFromString(p)
	if err != nil {
		return err
	}
	n, _, errno := syscall.Syscall(syscall.SYS_LLISTXATTR, uintptr(unsafe.Pointer(name)), 0, 0)
	if errno != 0 && errno != syscall.ENOTSUP {
		return errno
	}
	if n > 0 && errno == 0 {
		return fmt.Errorf("extended attributes/ACLs unsupported: %s", p)
	}
	return nil
}

func rejectMounts(root string) error {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1<<20)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 5 {
			return fmt.Errorf("invalid mountinfo")
		}
		// mountinfo escapes spaces, tabs, newlines and backslashes with octal.
		mount, err := strconv.Unquote(`"` + strings.ReplaceAll(fields[4], `"`, `\"`) + `"`)
		if err != nil {
			return err
		}
		if mount == root || strings.HasPrefix(mount, root+string(filepath.Separator)) {
			return fmt.Errorf("source contains mount point: %s", mount)
		}
	}
	return s.Err()
}

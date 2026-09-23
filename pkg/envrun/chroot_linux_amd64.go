package envrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const HelperCommand = "__plx-env-chroot-helper"

// ExperimentalChroot is only for trusted test payloads in a disposable Linux
// environment. Chroot is NOT a security boundary for hostile programs.
// Executable must be the trusted plx-env binary implementing HelperCommand.
type ExperimentalChroot struct{ Executable string }

type request struct {
	Directory   string `json:"directory"`
	ParentMount string `json:"parent_mount"`
}

func lockDirectory(p string) (*os.File, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("environment is busy: %w", err)
	}
	return f, nil
}

func (b ExperimentalChroot) Run(ctx context.Context, p Plan, s Streams) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("experimental chroot requires root with mount/chroot/setuid/setgid capabilities; no user fallback")
	}
	if p.lock == nil {
		return fmt.Errorf("execution requires directory lock")
	}
	parentMount, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		return err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	defer w.Close()
	cmd := exec.CommandContext(ctx, b.Executable, HelperCommand)
	cmd.Env = []string{}
	cmd.Stdin = s.Stdin
	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	cmd.ExtraFiles = []*os.File{r, p.lock}
	// Mounts never enter the caller's namespace. PID 1 exiting also terminates
	// payload descendants, so mounts and the execution lock cannot outlive run.
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWPID, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start isolated test helper (CAP_SYS_ADMIN required): %w", err)
	}
	r.Close()
	written := make(chan error, 1)
	go func() { err := json.NewEncoder(w).Encode(request{p.Directory, parentMount}); w.Close(); written <- err }()
	waitErr := cmd.Wait()
	writeErr := <-written
	if waitErr != nil {
		return fmt.Errorf("experimental chroot: %w", waitErr)
	}
	return writeErr
}

// RunHelper is an internal CLI entry point, not a standalone execution API.
// Never unshare/mount/chroot the caller's Go process: this runs in a new process
// whose namespaces were created by SysProcAttr before the Go runtime started.
func RunHelper() error {
	if os.Geteuid() != 0 || os.Getpid() != 1 {
		return fmt.Errorf("helper requires root inside a fresh PID namespace")
	}
	input := os.NewFile(3, "request")
	if input == nil {
		return fmt.Errorf("missing request pipe")
	}
	defer input.Close()
	var req request
	decoder := json.NewDecoder(io.LimitReader(input, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid helper request")
	}
	input.Close()
	selfMount, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		return err
	}
	if req.ParentMount == "" || selfMount == req.ParentMount {
		return fmt.Errorf("refusing to mount in caller namespace")
	}
	lock := os.NewFile(4, "environment-lock")
	if lock == nil {
		return fmt.Errorf("missing lock")
	}
	defer lock.Close()
	lockedInfo, err := lock.Stat()
	if err != nil {
		return err
	}
	root, err := cleanDirectory(req.Directory)
	if err != nil {
		return err
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !os.SameFile(lockedInfo, rootInfo) {
		return fmt.Errorf("source directory changed")
	}
	p, err := prepare(root)
	if err != nil {
		return err
	}
	// A private mount tree prevents propagation to any host/container mount.
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("private mount tree: %w", err)
	}
	if err := syscall.Mount(p.Rootfs, p.Rootfs, "", syscall.MS_BIND, ""); err != nil {
		return err
	}
	if err := syscall.Mount("", p.Rootfs, "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NODEV, ""); err != nil {
		return err
	}
	for _, m := range p.Placements {
		target := filepath.Join(p.Rootfs, filepath.FromSlash(strings.TrimPrefix(m.Target, "/")))
		if err := syscall.Mount(m.Directory, target, "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind %s: %w", m.Target, err)
		}
		if err := syscall.Mount("", target, "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_NOSUID|syscall.MS_NODEV, ""); err != nil {
			return err
		}
	}
	// Optional empty virtual directories are populated only inside this private
	// execution namespace. They are never persisted in the stopped rootfs.
	for _, name := range []string{"proc", "dev"} {
		target := filepath.Join(p.Rootfs, name)
		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.IsDir() {
			return fmt.Errorf("virtual directory %s must be an empty real directory", name)
		}
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			return fmt.Errorf("virtual directory %s must be empty", name)
		}
		if name == "proc" {
			if err := syscall.Mount("proc", target, "proc", syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
				return fmt.Errorf("mount private proc: %w", err)
			}
			continue
		}
		if err := syscall.Mount("tmpfs", target, "tmpfs", syscall.MS_NOSUID|syscall.MS_NOEXEC, "size=64k,mode=0755"); err != nil {
			return fmt.Errorf("mount private dev: %w", err)
		}
		null := filepath.Join(target, "null")
		if err := os.WriteFile(null, nil, 0600); err != nil {
			return err
		}
		if err := syscall.Mount("/dev/null", null, "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind null device: %w", err)
		}
		if err := syscall.Mount("", target, "", syscall.MS_REMOUNT|syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NOEXEC, ""); err != nil {
			return err
		}
	}
	d := p.Definition
	// Explicit Path bypasses os/exec's host PATH lookup. Chroot, credential and
	// chdir are applied in the child before exec. Args and Env are never shell text.
	cmd := &exec.Cmd{Path: d.Command[0], Args: d.Command, Env: p.Environment, Dir: d.Workdir,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		SysProcAttr: &syscall.SysProcAttr{Chroot: p.Rootfs, Credential: &syscall.Credential{Uid: d.UID, Gid: d.GID, Groups: []uint32{}}}}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apply definition / execute payload: %w", err)
	}
	return nil
}

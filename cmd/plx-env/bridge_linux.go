//go:build linux && amd64

package main

import (
	"PocketLinx/pkg/environment"
	"PocketLinx/pkg/wslbridge"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Native storage is deliberately restricted to known Linux filesystems.
// DrvFS/9p/NTFS and symlink ancestors are rejected before receiving data.
func nativeDirectory(dir string) error {
	if !wslbridge.LinuxPath(dir) {
		return fmt.Errorf("absolute Linux directory required")
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if real != dir {
		return fmt.Errorf("symlink in Linux directory")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	var st syscall.Statfs_t
	if err = syscall.Statfs(dir, &st); err != nil {
		return err
	}
	switch uint64(st.Type) {
	case 0xef53, 0x58465342, 0x9123683e:
		return nil
	}
	return fmt.Errorf("Linux-native ext4/xfs/btrfs storage required; filesystem type %#x", st.Type)
}
func runBridge(args []string, out io.Writer) error {
	if len(args) == 1 && args[0] == "check" {
		if _, err := fmt.Fprintln(out, "plx-env-wsl-bridge-v1 linux/amd64"); err != nil {
			return err
		}
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("invalid internal bridge operation")
	}
	dir := args[1]
	switch args[0] {
	case "restore":
		if len(args) != 3 || len(args[2]) != 64 {
			return fmt.Errorf("restore requires SHA-256")
		}
		if _, err := hex.DecodeString(args[2]); err != nil {
			return err
		}
		if !wslbridge.LinuxPath(dir) {
			return fmt.Errorf("invalid destination")
		}
		if err := nativeDirectory(filepath.Dir(dir)); err != nil {
			return err
		}
		if _, err := os.Lstat(dir); !os.IsNotExist(err) {
			return fmt.Errorf("destination exists or inaccessible")
		}
		f, err := os.CreateTemp(filepath.Dir(dir), ".plx-upload-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		defer f.Close()
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(os.Stdin, wslbridge.MaxTransferBytes+1))
		if err != nil {
			return err
		}
		if n > wslbridge.MaxTransferBytes {
			return fmt.Errorf("transfer too large")
		}
		if hex.EncodeToString(h.Sum(nil)) != args[2] {
			return fmt.Errorf("upload SHA-256 mismatch")
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = environment.Restore(f.Name(), dir, environment.Limits{}); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, args[2])
		return err
	case "save":
		if len(args) != 2 {
			return fmt.Errorf("invalid save arguments")
		}
		if err := nativeDirectory(dir); err != nil {
			return err
		}
		if err := nativeDirectory(filepath.Dir(dir)); err != nil {
			return err
		}
		scratch, err := os.MkdirTemp(filepath.Dir(dir), ".plx-download-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(scratch)
		bundle := filepath.Join(scratch, "environment.plxenv")
		if err = environment.Save(dir, bundle, environment.SaveOptions{Stopped: true}); err != nil {
			return err
		}
		f, err := os.Open(bundle)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			return err
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if _, err = fmt.Fprintf(out, "%x\n", h.Sum(nil)); err != nil {
			return err
		}
		_, err = io.Copy(out, f)
		return err
	case "run":
		if len(args) != 2 {
			return fmt.Errorf("invalid run arguments")
		}
		if err := nativeDirectory(dir); err != nil {
			return err
		}
		return run([]string{"run", dir}, out)
	}
	return fmt.Errorf("unknown bridge operation")
}

//go:build linux && amd64

// Verification utility only; does not implement storage, transport or execution.
package main

import (
	"PocketLinx/pkg/environment"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
)

type entry struct {
	Path         string
	Mode         uint32
	UID, GID     uint32
	SHA256, Link string
}

func snapshot(root string) ([]entry, error) {
	if _, err := environment.InspectDirectory(root); err != nil {
		return nil, err
	}
	var entries []entry
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		stat := info.Sys().(*syscall.Stat_t)
		e := entry{Path: filepath.ToSlash(rel), Mode: stat.Mode, UID: stat.Uid, GID: stat.Gid}
		if info.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return err
			}
			e.SHA256 = hex.EncodeToString(h.Sum(nil))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			e.Link, err = os.Readlink(p)
			if err != nil {
				return err
			}
		}
		entries = append(entries, e)
		return nil
	})
	return entries, err
}
func markers(root string) ([]string, error) {
	d, err := environment.LoadDefinition(filepath.Join(root, "environment.json"))
	if err != nil {
		return nil, err
	}
	if d.UID == 0 || d.GID == 0 || len(d.Volumes) != 1 {
		return nil, fmt.Errorf("requires non-root dedicated sample with one volume")
	}
	return []string{filepath.Join(root, "source", "roundtrip-delete-me"), filepath.Join(root, "volumes", d.Volumes[0].Name, "roundtrip-delete-me")}, nil
}
func run(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("snapshot ROOT | check ROOT EVIDENCE | seed ROOT | delete ROOT | execute ROOT CLI STATE | host ROLE")
	}
	op, root := args[0], args[1]
	switch op {
	case "denied":
		if len(args) != 3 || os.Geteuid() != 0 {
			return fmt.Errorf("denied check requires root and CLI path")
		}
		before, err := snapshot(root)
		if err != nil {
			return err
		}
		cmd := exec.Command(args[2], "run", root)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534, Groups: []uint32{}}}
		out, err := cmd.CombinedOutput()
		if _, ok := err.(*exec.ExitError); !ok || (!strings.Contains(string(out), "permission denied") && !strings.Contains(string(out), "requires root")) {
			return fmt.Errorf("expected explicit non-root error, got %v %s", err, out)
		}
		after, err := snapshot(root)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("failed execution changed environment")
		}
		fmt.Print(string(out))
		return nil
	case "host":
		if root != "linux-vm" && root != "linux-physical" && root != "wsl" && root != "smoke" {
			return fmt.Errorf("explicit host role required")
		}
		kernel, err := os.ReadFile("/proc/sys/kernel/osrelease")
		if err != nil {
			return err
		}
		mounts, err := os.ReadFile("/proc/self/mountinfo")
		if err != nil {
			return err
		}
		restricted := strings.Contains(strings.ToLower(string(kernel)), "microsoft") || strings.Contains(string(mounts), " - overlay ") || os.Getenv("container") != ""
		for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
			if _, err := os.Stat(p); err == nil {
				restricted = true
			}
		}
		if strings.HasPrefix(root, "linux-") && restricted {
			return fmt.Errorf("independent Linux role rejected: WSL/container detected")
		}
		host, _ := os.Hostname()
		fmt.Printf("role=%s host=%s kernel=%s", root, host, kernel)
		return nil
	case "snapshot", "check":
		got, err := snapshot(root)
		if err != nil {
			return err
		}
		if op == "snapshot" {
			return json.NewEncoder(os.Stdout).Encode(got)
		}
		if len(args) != 3 {
			return fmt.Errorf("check requires evidence JSON")
		}
		var b []byte
		if args[2] == "-" {
			b, err = io.ReadAll(io.LimitReader(os.Stdin, 32<<20))
		} else {
			b, err = os.ReadFile(args[2])
		}
		if err != nil {
			return err
		}
		var expected []entry
		if err = json.Unmarshal(b, &expected); err != nil {
			return err
		}
		if !reflect.DeepEqual(got, expected) {
			return fmt.Errorf("file contents/types/modes/uid/gid/links or Definition differ")
		}
		return nil
	case "seed", "delete":
		paths, err := markers(root)
		if err != nil {
			return err
		}
		for _, p := range paths {
			if op == "seed" {
				f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				_, err = f.WriteString("must disappear after receiver edit\n")
				f.Close()
				if err != nil {
					return err
				}
			}
			if op == "delete" {
				if err = os.Remove(p); err != nil {
					return err
				}
			}
		}
		return nil
	case "execute", "expected":
		if len(args) != 4 {
			return fmt.Errorf("execute requires CLI and original|updated|returned")
		}
		d, err := environment.LoadDefinition(filepath.Join(root, "environment.json"))
		if err != nil {
			return err
		}
		paths, err := markers(root)
		if err != nil {
			return err
		}
		value, suffix := "persistent-data", ""
		switch args[3] {
		case "original":
		case "updated":
			value, suffix = "updated-data", "edited\n"
		case "returned":
			value, suffix = "runtime-write", "edited\n"
		default:
			return fmt.Errorf("invalid sample state")
		}
		if args[3] != "original" {
			for _, p := range paths {
				if _, err := os.Lstat(p); !os.IsNotExist(err) {
					return fmt.Errorf("deleted marker still exists: %s", p)
				}
			}
		}
		expected := fmt.Sprintf("%s|%s|%s|uid=%d|gid=%d\n%s", d.Env["GREETING"], d.Workdir, value, d.UID, d.GID, suffix)
		if op == "expected" {
			fmt.Print(expected)
			return nil
		}
		cmd := exec.Command(args[2], "run", root)
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		if err != nil {
			return err
		}
		if string(output) != expected {
			return fmt.Errorf("Definition output mismatch: got %q expected %q", output, expected)
		}
		if args[3] != "original" {
			b, err := os.ReadFile(filepath.Join(root, "source", "generated.txt"))
			if err != nil || string(b) != "source-write\n" {
				return fmt.Errorf("runtime source write missing")
			}
		}
		fmt.Print(string(output))
		return nil
	}
	return fmt.Errorf("unknown verification operation")
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

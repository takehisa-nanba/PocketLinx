// Test fixture utility for a dedicated Alpine WSL distro; not a product command.
package main

import (
	"PocketLinx/pkg/environment"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func main() {
	if err := prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func prepare() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("usage: fixture create|edit ROOT SAMPLE_DIRECTORY")
	}
	mode, root, sample := os.Args[1], os.Args[2], os.Args[3]
	defFile := filepath.Join(sample, "environment.json")
	if mode != "create" {
		defFile = filepath.Join(root, "environment.json")
	}
	d, err := environment.LoadDefinition(defFile)
	if err != nil {
		return err
	}
	if len(d.Volumes) != 1 {
		return fmt.Errorf("dedicated single-volume sample required")
	}
	scriptRel := strings.TrimPrefix(d.Command[1], d.Source.Target+"/")
	dataRel := strings.TrimPrefix(d.Env["DATA_FILE"], d.Volumes[0].Target+"/")
	script := filepath.Join(root, "source", scriptRel)
	data := filepath.Join(root, "volumes", d.Volumes[0].Name, dataRel)
	if mode == "exit" {
		return os.WriteFile(script, []byte("#!/bin/sh\nexit 37\n"), 0755)
	}
	if mode == "edit" {
		f, err := os.OpenFile(script, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err = f.WriteString("\nprintf 'edited\\n'\nprintf 'runtime-write\\n' > \"$DATA_FILE\"\nprintf 'source-write\\n' > generated.txt\n"); err != nil {
			return err
		}
		return os.WriteFile(data, []byte("updated-data\n"), 0644)
	}
	if mode != "create" {
		return fmt.Errorf("unknown mode")
	}
	if err = os.Mkdir(root, 0755); err != nil {
		return err
	}
	for _, dir := range []string{"rootfs/bin", "rootfs/lib", "source", path.Join("volumes", d.Volumes[0].Name), path.Join("rootfs", d.Source.Target), path.Join("rootfs", d.Volumes[0].Target)} {
		if err = os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			return err
		}
	}
	for _, pair := range [][2]string{{"/bin/busybox", filepath.Join(root, "rootfs/bin/busybox")}, {"/lib/ld-musl-x86_64.so.1", filepath.Join(root, "rootfs/lib/ld-musl-x86_64.so.1")}, {defFile, filepath.Join(root, "environment.json")}, {filepath.Join(sample, "hello.sh"), script}} {
		b, err := os.ReadFile(pair[0])
		if err != nil {
			return err
		}
		if err = os.WriteFile(pair[1], b, 0755); err != nil {
			return err
		}
	}
	if err = os.Symlink("busybox", filepath.Join(root, "rootfs/bin/sh")); err != nil {
		return err
	}
	if err = os.WriteFile(data, []byte("persistent-data\n"), 0644); err != nil {
		return err
	}
	for _, dir := range []string{"source", "volumes"} {
		if err = filepath.Walk(filepath.Join(root, dir), func(p string, _ os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			return os.Chown(p, int(d.UID), int(d.GID))
		}); err != nil {
			return err
		}
	}
	return nil
}

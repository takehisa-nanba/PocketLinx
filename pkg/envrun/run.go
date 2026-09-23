// Package envrun reproduces execution settings separately from bundle storage.
// The current backend is an experimental harness for trusted test environments,
// not a production security boundary.
package envrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"PocketLinx/pkg/environment"
)

type Streams struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}
type Placement struct{ Directory, Target string }
type Plan struct {
	Directory   string
	Rootfs      string
	Definition  environment.Definition
	Placements  []Placement
	Environment []string
	lock        *os.File
}

// Backend must preserve argv, the explicit environment, workdir and credentials.
// An unsupported capability must fail; it must never fall back to host execution.
type Backend interface {
	Run(context.Context, Plan, Streams) error
}

// Run holds the same directory flock used by Save for the complete execution.
func Run(ctx context.Context, directory string, backend Backend, streams Streams) error {
	if backend == nil {
		return fmt.Errorf("execution backend required")
	}
	root, err := cleanDirectory(directory)
	if err != nil {
		return err
	}
	lock, err := lockDirectory(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	plan, err := prepare(root)
	if err != nil {
		return err
	}
	plan.lock = lock
	return backend.Run(ctx, plan, streams)
}

func prepare(root string) (Plan, error) {
	p := Plan{Directory: root, Rootfs: filepath.Join(root, "rootfs")}
	d, err := environment.InspectDirectory(root)
	if err != nil {
		return p, err
	}
	if d.Version != 2 {
		return p, fmt.Errorf("run requires definition version 2 with explicit placements; version 1 can still be saved/restored")
	}
	p.Definition = d
	p.Placements = append(p.Placements, Placement{Directory: filepath.Join(root, "source"), Target: d.Source.Target})
	for _, v := range d.Volumes {
		p.Placements = append(p.Placements, Placement{Directory: filepath.Join(root, "volumes", v.Name), Target: v.Target})
	}
	for _, m := range p.Placements {
		if _, err := cleanDirectory(m.Directory); err != nil {
			return p, fmt.Errorf("placement source %s: %w", m.Directory, err)
		}
		target := filepath.Join(p.Rootfs, filepath.FromSlash(strings.TrimPrefix(m.Target, "/")))
		if _, err := cleanDirectory(target); err != nil {
			return p, fmt.Errorf("placement target %s must be an existing real directory: %w", m.Target, err)
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return p, err
		}
		if len(entries) != 0 {
			return p, fmt.Errorf("placement target must be empty: %s", m.Target)
		}
	}
	command := d.Command[0]
	if !strings.HasPrefix(command, "/") || filepath.ToSlash(filepath.Clean(command)) != command || strings.Contains(command, "\\") {
		return p, fmt.Errorf("run requires an absolute container command path (no host PATH lookup)")
	}
	exe, err := resolve(p, command)
	if err != nil {
		return p, fmt.Errorf("command: %w", err)
	}
	info, err := os.Stat(exe)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return p, fmt.Errorf("command does not exist or is not executable: %s", command)
	}
	workdir, err := resolve(p, d.Workdir)
	if err != nil {
		return p, fmt.Errorf("workdir: %w", err)
	}
	info, err = os.Stat(workdir)
	if err != nil || !info.IsDir() {
		return p, fmt.Errorf("workdir does not exist: %s", d.Workdir)
	}
	// A non-nil, possibly empty slice prevents os/exec from inheriting host env.
	p.Environment = make([]string, 0, len(d.Env))
	for k, v := range d.Env {
		p.Environment = append(p.Environment, k+"="+v)
	}
	sort.Strings(p.Environment)
	return p, nil
}

func resolve(p Plan, containerPath string) (string, error) {
	base := p.Rootfs
	relative := strings.TrimPrefix(containerPath, "/")
	for _, m := range p.Placements {
		if containerPath == m.Target || strings.HasPrefix(containerPath, m.Target+"/") {
			base = m.Directory
			relative = strings.TrimPrefix(strings.TrimPrefix(containerPath, m.Target), "/")
			break
		}
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	if resolved != base && !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return "", fmt.Errorf("path leaves its component")
	}
	return resolved, nil
}

func cleanDirectory(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	if real != abs {
		return "", fmt.Errorf("symlink in directory path: %s", p)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", p)
	}
	return abs, nil
}

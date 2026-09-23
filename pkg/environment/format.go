// Package environment saves and restores stopped, explicitly prepared development
// environments. It does not start containers or execute anything from a bundle.
package environment

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

const Format = "pocketlinx.environment.v1"

// Definition contains portable settings only. Host bindings and secret values
// must be supplied separately by a future runtime adapter.
type Definition struct {
	Version int               `json:"version"`
	OS      string            `json:"os"`
	Arch    string            `json:"arch"`
	Command []string          `json:"command"`
	Workdir string            `json:"workdir"`
	UID     uint32            `json:"uid"`
	GID     uint32            `json:"gid"`
	Env     map[string]string `json:"env,omitempty"`
}

// Entry describes exactly one tar member. Paths always use '/' separators.
type Entry struct {
	Path   string `json:"path"`
	Type   string `json:"type"` // directory, file, symlink
	Mode   uint32 `json:"mode"` // permission bits only
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Mtime  int64  `json:"mtime"` // seconds since Unix epoch
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

type Manifest struct {
	Format  string  `json:"format"`
	Entries []Entry `json:"entries"`
}

// Limits bound resource use before writing payload data. Zero values select
// defaults. Limits apply to uncompressed bytes; v1 uses uncompressed tar.
type Limits struct {
	MaxBytes         int64
	MaxEntries       int
	MaxManifestBytes int64
}

func (l Limits) normalized() (Limits, error) {
	if l.MaxBytes == 0 {
		l.MaxBytes = 8 << 30
	}
	if l.MaxEntries == 0 {
		l.MaxEntries = 100000
	}
	if l.MaxManifestBytes == 0 {
		l.MaxManifestBytes = 16 << 20
	}
	if l.MaxBytes < 0 || l.MaxEntries < 1 || l.MaxManifestBytes < 1 {
		return l, fmt.Errorf("limits must be positive")
	}
	return l, nil
}

type SaveOptions struct {
	// Stopped is the caller's assertion that all writers and processes using the
	// source have stopped. This package cannot discover arbitrary container PIDs.
	Stopped bool
	Limits  Limits
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (d Definition) validate() error {
	if d.UID == ^uint32(0) || d.GID == ^uint32(0) {
		return fmt.Errorf("invalid uid/gid sentinel")
	}
	if d.Version != 1 || d.OS != "linux" || d.Arch != "amd64" {
		return fmt.Errorf("unsupported definition: require version 1, linux/amd64")
	}
	if len(d.Command) == 0 || d.Command[0] == "" {
		return fmt.Errorf("command must be a nonempty argument array")
	}
	if !strings.HasPrefix(d.Workdir, "/") || path.Clean(d.Workdir) != d.Workdir || strings.ContainsRune(d.Workdir, 0) {
		return fmt.Errorf("workdir must be a clean absolute container path")
	}
	for _, a := range d.Command {
		if strings.ContainsRune(a, 0) {
			return fmt.Errorf("NUL in argument")
		}
	}
	for k, v := range d.Env {
		if !envName.MatchString(k) || strings.ContainsRune(v, 0) {
			return fmt.Errorf("invalid environment variable %q", k)
		}
	}
	return nil
}

func decodeJSON(r io.Reader, v interface{}) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra interface{}
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

func validPath(p string) bool {
	return p != "" && p != "." && p != ".." && !strings.HasPrefix(p, "/") &&
		!strings.HasPrefix(p, "../") && path.Clean(p) == p && !strings.ContainsAny(p, "\\\x00\r\n")
}

func component(p string) string { return strings.SplitN(p, "/", 2)[0] }

func validateManifest(m Manifest, limits Limits) error {
	if m.Format != Format {
		return fmt.Errorf("unsupported bundle format %q", m.Format)
	}
	if len(m.Entries) > limits.MaxEntries {
		return fmt.Errorf("too many entries")
	}
	seen := map[string]Entry{}
	var total int64
	for _, e := range m.Entries {
		if e.UID == ^uint32(0) || e.GID == ^uint32(0) {
			return fmt.Errorf("invalid uid/gid: %s", e.Path)
		}
		if !validPath(e.Path) {
			return fmt.Errorf("unsafe path %q", e.Path)
		}
		if _, ok := seen[e.Path]; ok {
			return fmt.Errorf("duplicate path %q", e.Path)
		}
		c := component(e.Path)
		if c != "rootfs" && c != "source" && c != "volumes" && e.Path != "environment.json" {
			return fmt.Errorf("unknown component %q", c)
		}
		if e.Mode > 01777 || e.Size < 0 || (e.Mode&01000 != 0 && e.Type != "directory") {
			return fmt.Errorf("unsupported attributes: %s", e.Path)
		}
		if parent := path.Dir(e.Path); parent != "." {
			if p, ok := seen[parent]; !ok || p.Type != "directory" {
				return fmt.Errorf("missing or non-directory parent: %s", e.Path)
			}
		}
		switch e.Type {
		case "directory":
			if e.Size != 0 || e.SHA256 != "" || e.Link != "" {
				return fmt.Errorf("invalid directory %s", e.Path)
			}
		case "file":
			digest, err := hex.DecodeString(e.SHA256)
			if err != nil || len(digest) != 32 || strings.ToLower(e.SHA256) != e.SHA256 || e.Link != "" {
				return fmt.Errorf("invalid file %s", e.Path)
			}
			if e.Size > limits.MaxBytes-total {
				return fmt.Errorf("payload exceeds byte limit")
			}
			total += e.Size
		case "symlink":
			if e.Size != 0 || e.SHA256 != "" || e.Link == "" || strings.ContainsAny(e.Link, "\\\x00\r\n") || path.IsAbs(e.Link) {
				return fmt.Errorf("unsupported symlink %s (only contained relative links are supported)", e.Path)
			}
			target := path.Clean(path.Join(path.Dir(e.Path), e.Link))
			if component(target) != c || target == c {
				return fmt.Errorf("symlink leaves its component: %s", e.Path)
			}
		default:
			return fmt.Errorf("unsupported entry type %q", e.Type)
		}
		// Virtual filesystems must be empty in a prepared, stopped rootfs.
		for _, virtual := range []string{"rootfs/proc/", "rootfs/sys/", "rootfs/dev/"} {
			if strings.HasPrefix(e.Path, virtual) {
				return fmt.Errorf("virtual filesystem is not empty: %s", e.Path)
			}
		}
		seen[e.Path] = e
	}
	for _, p := range []string{"rootfs", "source", "volumes"} {
		if seen[p].Type != "directory" {
			return fmt.Errorf("required directory missing: %s", p)
		}
	}
	if e := seen["environment.json"]; e.Type != "file" || e.Size > 1<<20 {
		return fmt.Errorf("missing or oversized environment.json")
	}
	for _, e := range m.Entries {
		if e.Type == "symlink" {
			// Resolve link chains virtually, including '..' after another link.
			// Lexical cleaning alone cannot establish containment.
			stack := strings.Split(path.Dir(e.Path), "/")
			pending := strings.Split(e.Link, "/")
			links := 0
			for len(pending) > 0 {
				part := pending[0]
				pending = pending[1:]
				if part == "" || part == "." {
					continue
				}
				if part == ".." {
					if len(stack) <= 1 {
						return fmt.Errorf("symlink chain escapes component: %s", e.Path)
					}
					stack = stack[:len(stack)-1]
					continue
				}
				candidate := strings.Join(append(append([]string{}, stack...), part), "/")
				if next, ok := seen[candidate]; ok && next.Type == "symlink" {
					links++
					if links > 40 {
						return fmt.Errorf("symlink loop: %s", e.Path)
					}
					pending = append(strings.Split(next.Link, "/"), pending...)
				} else {
					stack = append(stack, part)
				}
			}
		}
	}
	return nil
}

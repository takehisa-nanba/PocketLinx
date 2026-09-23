package environment

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// Save writes a complete v1 bundle without overwriting an existing file. Source
// must be a prepared, stopped directory with environment.json, rootfs/, source/
// and volumes/. It is never modified (filesystem access times may change).
func Save(source, output string, opts SaveOptions) error {
	if err := supported(); err != nil {
		return err
	}
	if !opts.Stopped {
		return fmt.Errorf("source must be stopped; explicit stopped assertion required")
	}
	limits, err := opts.Limits.normalized()
	if err != nil {
		return err
	}
	source, err = canonicalDirectory(source)
	if err != nil {
		return err
	}
	output, err = newDestination(output)
	if err != nil {
		return err
	}
	if within(source, output) {
		return fmt.Errorf("bundle output must be outside source")
	}
	lock, err := lockDirectory(source)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := rejectMounts(source); err != nil {
		return err
	}
	m, before, err := inventory(source, limits)
	if err != nil {
		return err
	}
	if err := readDefinition(filepath.Join(source, "environment.json")); err != nil {
		return err
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if int64(len(manifest)) > limits.MaxManifestBytes {
		return fmt.Errorf("manifest too large")
	}
	tmp, err := os.CreateTemp(filepath.Dir(output), ".plx-save-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { tmp.Close(); os.Remove(tmpName) }()
	tw := tar.NewWriter(tmp)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		return err
	}
	if _, err := tw.Write(manifest); err != nil {
		return err
	}
	for _, e := range m.Entries {
		h := header(e)
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if e.Type == "file" {
			f, err := openRegular(filepath.Join(source, filepath.FromSlash(e.Path)))
			if err != nil {
				return err
			}
			info, statErr := f.Stat()
			if statErr != nil || !os.SameFile(before[e.Path], info) {
				f.Close()
				return fmt.Errorf("source changed: %s", e.Path)
			}
			hash := sha256.New()
			n, copyErr := io.CopyN(io.MultiWriter(tw, hash), f, e.Size)
			after, statErr := f.Stat()
			f.Close()
			if copyErr != nil || statErr != nil || n != e.Size || !unchanged(before[e.Path], after) || hex.EncodeToString(hash.Sum(nil)) != e.SHA256 {
				return fmt.Errorf("source changed or could not be read: %s", e.Path)
			}
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	// Repeat inventory to detect changes in names, links, attributes and bytes.
	// This is not a substitute for stopping external writers.
	after, _, err := inventory(source, limits)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(m, after) {
		return fmt.Errorf("source changed during save")
	}
	if err := rejectMounts(source); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// A hard link publishes the complete file atomically, without replacement.
	if err := os.Link(tmpName, output); err != nil {
		return fmt.Errorf("publish bundle without overwrite: %w", err)
	}
	return nil
}

// Restore validates every entry and digest in a private sibling directory before
// publishing destination atomically. Destination must not exist. No payload is
// executed. The parent must be trusted and not moved/replaced during the call.
func Restore(bundle, destination string, limits Limits) error {
	if err := supported(); err != nil {
		return err
	}
	limits, err := limits.normalized()
	if err != nil {
		return err
	}
	destination, err = newDestination(destination)
	if err != nil {
		return err
	}
	f, err := openRegular(bundle)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("bundle must be a regular file")
	}
	tr := tar.NewReader(f)
	h, err := tr.Next()
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if h.Name != "manifest.json" || h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > limits.MaxManifestBytes {
		return fmt.Errorf("invalid or oversized manifest header")
	}
	var m Manifest
	if err := decodeJSON(tr, &m); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if err := validateManifest(m, limits); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".plx-restore-*")
	if err != nil {
		return err
	}
	defer removeStage(stage)
	for _, e := range m.Entries {
		h, err := tr.Next()
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Path, err)
		}
		if err := matches(h, e); err != nil {
			return err
		}
		p := filepath.Join(stage, filepath.FromSlash(e.Path))
		switch e.Type {
		case "directory":
			if err := os.Mkdir(p, 0700); err != nil {
				return err
			}
		case "file":
			file, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, copyErr := io.Copy(io.MultiWriter(file, hash), tr)
			syncErr := file.Sync()
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if syncErr != nil {
				return syncErr
			}
			if closeErr != nil {
				return closeErr
			}
			if hex.EncodeToString(hash.Sum(nil)) != e.SHA256 {
				return fmt.Errorf("checksum mismatch: %s", e.Path)
			}
		case "symlink": // Created only after all regular files have been written.
		}
	}
	endStart, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if _, err := tr.Next(); err != io.EOF {
		return fmt.Errorf("extra or malformed tar entry: %v", err)
	}
	// Reject truncation (tar.Reader permits missing end blocks) and appended data.
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	lastSize := m.Entries[len(m.Entries)-1].Size
	padding := (512 - lastSize%512) % 512
	if offset != info.Size() || offset-endStart != padding+1024 {
		return fmt.Errorf("trailing data or missing tar terminator")
	}
	end := make([]byte, 1024)
	if _, err := f.ReadAt(end, offset-1024); err != nil || !bytes.Equal(end, make([]byte, 1024)) {
		return fmt.Errorf("missing tar terminator")
	}
	if err := readDefinition(filepath.Join(stage, "environment.json")); err != nil {
		return err
	}
	for _, e := range m.Entries {
		if e.Type == "symlink" {
			if err := os.Symlink(e.Link, filepath.Join(stage, filepath.FromSlash(e.Path))); err != nil {
				return err
			}
		}
	}
	// Children first: a restored read-only parent must not block its children.
	for i := len(m.Entries) - 1; i >= 0; i-- {
		e := m.Entries[i]
		if err := applyMetadata(filepath.Join(stage, filepath.FromSlash(e.Path)), e); err != nil {
			return err
		}
	}
	return publishDirectory(stage, destination)
}

func readDefinition(p string) error {
	f, err := openRegular(p)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return fmt.Errorf("invalid environment.json")
	}
	var d Definition
	if err := decodeJSON(io.LimitReader(f, (1<<20)+1), &d); err != nil {
		return fmt.Errorf("environment.json: %w", err)
	}
	return d.validate()
}

func canonicalDirectory(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	if abs != real {
		return "", fmt.Errorf("symlink in directory path: %s", p)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", p)
	}
	return real, nil
}

func newDestination(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("destination is required")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if _, err := canonicalDirectory(filepath.Dir(abs)); err != nil {
		return "", err
	}
	if _, err := os.Lstat(abs); !os.IsNotExist(err) {
		return "", fmt.Errorf("destination exists or is inaccessible: %s", p)
	}
	return abs, nil
}

func within(root, p string) bool {
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

func unchanged(a, b os.FileInfo) bool {
	return b != nil && os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}

func inventory(root string, limits Limits) (Manifest, map[string]os.FileInfo, error) {
	m := Manifest{Format: Format}
	stats := make(map[string]os.FileInfo)
	var total int64
	err := filepath.WalkDir(root, func(p string, de fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := de.Info()
		if err != nil {
			return err
		}
		if err := checkAttributes(p, info); err != nil {
			return err
		}
		u, g := ownership(info)
		e := Entry{Path: rel, Mode: uint32(info.Mode().Perm()), UID: u, GID: g, Mtime: info.ModTime().Unix()}
		if info.Mode()&os.ModeSticky != 0 {
			e.Mode |= 01000
		}
		switch {
		case info.IsDir():
			e.Type = "directory"
		case info.Mode().IsRegular():
			e.Type = "file"
			e.Size = info.Size()
			if e.Size > limits.MaxBytes-total {
				return fmt.Errorf("payload exceeds byte limit")
			}
			total += e.Size
			f, err := openRegular(p)
			if err != nil {
				return err
			}
			opened, err := f.Stat()
			if err != nil || !unchanged(info, opened) {
				f.Close()
				return fmt.Errorf("source changed: %s", rel)
			}
			h := sha256.New()
			n, copyErr := io.CopyN(h, f, e.Size)
			after, statErr := f.Stat()
			f.Close()
			if copyErr != nil || statErr != nil || n != e.Size || !unchanged(info, after) {
				return fmt.Errorf("source changed or unreadable: %s", rel)
			}
			e.SHA256 = hex.EncodeToString(h.Sum(nil))
		case info.Mode()&os.ModeSymlink != 0:
			e.Type = "symlink"
			e.Link, err = os.Readlink(p)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported special file: %s", rel)
		}
		m.Entries = append(m.Entries, e)
		stats[rel] = info
		if len(m.Entries) > limits.MaxEntries {
			return fmt.Errorf("too many entries")
		}
		return nil
	})
	if err == nil {
		err = validateManifest(m, limits)
	}
	return m, stats, err
}

func header(e Entry) *tar.Header {
	h := &tar.Header{Name: e.Path, Mode: int64(e.Mode), Uid: int(e.UID), Gid: int(e.GID), Size: e.Size, Linkname: e.Link, ModTime: time.Unix(e.Mtime, 0), Format: tar.FormatPAX}
	switch e.Type {
	case "file":
		h.Typeflag = tar.TypeReg
	case "directory":
		h.Typeflag = tar.TypeDir
	case "symlink":
		h.Typeflag = tar.TypeSymlink
	}
	return h
}

func matches(h *tar.Header, e Entry) error {
	want := header(e)
	if h.Name != want.Name || h.Typeflag != want.Typeflag || h.Mode != want.Mode || h.Uid != want.Uid || h.Gid != want.Gid || h.Size != want.Size || h.Linkname != want.Linkname || h.ModTime.Unix() != e.Mtime || len(h.Xattrs) != 0 {
		return fmt.Errorf("tar header disagrees with manifest: %s", e.Path)
	}
	for k := range h.PAXRecords {
		if k != "path" && k != "linkpath" && k != "mtime" && k != "uid" && k != "gid" && k != "size" {
			return fmt.Errorf("unsupported PAX field %q", k)
		}
	}
	return nil
}

func removeStage(stage string) {
	// Metadata restoration may have made directories read-only. WalkDir does
	// not follow links; make directories traversable before removing our stage.
	_ = filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0700)
		}
		return nil
	})
	_ = os.RemoveAll(stage)
}

package environment

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "env")
	for _, p := range []string{"rootfs/bin", "rootfs/tmp", "source", "volumes/data"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	d := Definition{Version: 1, OS: "linux", Arch: "amd64", Command: []string{"/bin/sh", "/workspace/hello.sh", "argument with spaces", ""}, Workdir: "/workspace", UID: 1000, GID: 1000, Env: map[string]string{"GREETING": "hello $() ' \"\n"}}
	b, _ := json.Marshal(d)
	for p, content := range map[string][]byte{"environment.json": b, "source/hello.sh": []byte("#!/bin/sh\nprintf '%s\\n' hello\n"), "volumes/data/value.txt": []byte("initial\n"), "rootfs/bin/tool": []byte("binary\x00data"), "source/empty": {}} {
		if err := os.WriteFile(filepath.Join(root, p), content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(root, "source/hello.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "rootfs/tmp"), 0777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tool", filepath.Join(root, "rootfs/bin/link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "source/dangling")); err != nil {
		t.Fatal(err)
	}
	return root
}

func saved(t *testing.T, root string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "environment.plxenv")
	if err := Save(root, p, SaveOptions{Stopped: true}); err != nil {
		t.Fatal(err)
	}
	return p
}

func assertUnpublished(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Fatalf("destination published: %v", err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".plx-restore-*"))
	if len(left) > 0 {
		t.Fatalf("staging leaked: %v", left)
	}
}

func TestRoundTripAndChangedResave(t *testing.T) {
	root := fixture(t)
	limits, _ := (Limits{}).normalized()
	before, _, err := inventory(root, limits)
	if err != nil {
		t.Fatal(err)
	}
	bundle := saved(t, root)
	dest := filepath.Join(t.TempDir(), "restored")
	if err := Restore(bundle, dest, Limits{}); err != nil {
		t.Fatal(err)
	}
	after, _, err := inventory(dest, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("round trip metadata/content mismatch\nbefore=%+v\nafter=%+v", before, after)
	}
	original, _, _ := inventory(root, limits)
	if !reflect.DeepEqual(before, original) {
		t.Fatal("save mutated source")
	}
	if err := os.WriteFile(filepath.Join(dest, "source/hello.sh"), []byte("#!/bin/sh\necho changed\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dest, "source/empty")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "volumes/data/value.txt"), []byte("changed data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second := saved(t, dest)
	back := filepath.Join(t.TempDir(), "back")
	if err := Restore(second, back, Limits{}); err != nil {
		t.Fatal(err)
	}
	modified, _, _ := inventory(dest, limits)
	again, _, _ := inventory(back, limits)
	if !reflect.DeepEqual(modified, again) {
		t.Fatal("changed environment did not survive second round trip")
	}
}

func TestSaveGuards(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string)
		opts    SaveOptions
	}{
		{"stopped required", func(*testing.T, string) {}, SaveOptions{}},
		{"absolute link", func(t *testing.T, p string) { must(t, os.Symlink("/etc/passwd", filepath.Join(p, "source/escape"))) }, SaveOptions{Stopped: true}},
		{"relative escape", func(t *testing.T, p string) { must(t, os.Symlink("../../outside", filepath.Join(p, "source/escape"))) }, SaveOptions{Stopped: true}},
		{"fifo", func(t *testing.T, p string) { must(t, syscall.Mkfifo(filepath.Join(p, "source/fifo"), 0600)) }, SaveOptions{Stopped: true}},
		{"setuid", func(t *testing.T, p string) {
			must(t, os.Chmod(filepath.Join(p, "source/hello.sh"), 0755|os.ModeSetuid))
		}, SaveOptions{Stopped: true}},
		{"virtual filesystem", func(t *testing.T, p string) {
			must(t, os.MkdirAll(filepath.Join(p, "rootfs/proc"), 0755))
			must(t, os.WriteFile(filepath.Join(p, "rootfs/proc/secret"), []byte("no"), 0600))
		}, SaveOptions{Stopped: true}},
		{"unknown root", func(t *testing.T, p string) { must(t, os.WriteFile(filepath.Join(p, "unexpected"), nil, 0600)) }, SaveOptions{Stopped: true}},
		{"invalid definition", func(t *testing.T, p string) {
			must(t, os.WriteFile(filepath.Join(p, "environment.json"), []byte(`{"version":99}`), 0600))
		}, SaveOptions{Stopped: true}},
		{"byte limit", func(*testing.T, string) {}, SaveOptions{Stopped: true, Limits: Limits{MaxBytes: 1}}},
		{"entry limit", func(*testing.T, string) {}, SaveOptions{Stopped: true, Limits: Limits{MaxEntries: 1}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			tc.prepare(t, root)
			out := filepath.Join(t.TempDir(), "out")
			if err := Save(root, out, tc.opts); err == nil {
				t.Fatal("save accepted invalid source")
			}
			if _, err := os.Lstat(out); !os.IsNotExist(err) {
				t.Fatal("published failed save")
			}
			left, _ := filepath.Glob(filepath.Join(filepath.Dir(out), ".plx-save-*"))
			if len(left) > 0 {
				t.Fatal("temporary file leaked")
			}
		})
	}
}

func TestNoOverwriteAndUnsafeDestinations(t *testing.T) {
	root := fixture(t)
	bundle := saved(t, root)
	original, err := os.ReadFile(bundle)
	must(t, err)
	if err := Save(root, bundle, SaveOptions{Stopped: true}); err == nil {
		t.Fatal("overwrote bundle")
	}
	current, _ := os.ReadFile(bundle)
	if !bytes.Equal(original, current) {
		t.Fatal("existing bundle changed")
	}
	if err := Save(root, filepath.Join(root, "bundle"), SaveOptions{Stopped: true}); err == nil {
		t.Fatal("accepted output in source")
	}
	for _, dest := range []string{root, t.TempDir(), bundle} {
		if err := Restore(bundle, dest, Limits{}); err == nil {
			t.Fatal("overwrote destination")
		}
	}
	parent := t.TempDir()
	link := filepath.Join(parent, "link")
	must(t, os.Symlink(t.TempDir(), link))
	if err := Restore(bundle, filepath.Join(link, "dest"), Limits{}); err == nil {
		t.Fatal("accepted symlink parent")
	}
	if err := Save(link, filepath.Join(t.TempDir(), "out"), SaveOptions{Stopped: true}); err == nil {
		t.Fatal("accepted symlink source")
	}
	// Exercise the publish primitive itself: a destination created after
	// preflight must still never be replaced, even if it is an empty directory.
	stage := filepath.Join(parent, "stage")
	target := filepath.Join(parent, "target")
	must(t, os.Mkdir(stage, 0700))
	must(t, os.Mkdir(target, 0700))
	if err := publishDirectory(stage, target); err == nil {
		t.Fatal("publish replaced concurrent destination")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type member struct {
	h    tar.Header
	data []byte
}

func readBundle(t *testing.T, p string) (Manifest, []member) {
	t.Helper()
	f, err := os.Open(p)
	must(t, err)
	defer f.Close()
	r := tar.NewReader(f)
	_, err = r.Next()
	must(t, err)
	var m Manifest
	must(t, json.NewDecoder(r).Decode(&m))
	var entries []member
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		must(t, err)
		data, err := io.ReadAll(r)
		must(t, err)
		entries = append(entries, member{*h, data})
	}
	return m, entries
}
func writeBundle(t *testing.T, m Manifest, entries []member) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bad.plxenv")
	f, err := os.Create(p)
	must(t, err)
	w := tar.NewWriter(f)
	b, _ := json.Marshal(m)
	must(t, w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Typeflag: tar.TypeReg, Size: int64(len(b))}))
	_, err = w.Write(b)
	must(t, err)
	for _, e := range entries {
		must(t, w.WriteHeader(&e.h))
		_, err = w.Write(e.data)
		must(t, err)
	}
	must(t, w.Close())
	must(t, f.Close())
	return p
}

func TestRejectMaliciousOrCorruptBundle(t *testing.T) {
	bundle := saved(t, fixture(t))
	for _, kind := range []string{"checksum", "traversal", "absolute", "duplicate", "symlink parent", "hardlink", "unknown format", "header mismatch", "extra member", "invalid definition", "missing member", "link chain escape", "symlink cycle"} {
		t.Run(kind, func(t *testing.T) {
			m, entries := readBundle(t, bundle)
			switch kind {
			case "checksum":
				for i := range entries {
					if entries[i].h.Name == "source/hello.sh" {
						entries[i].data[0] = 'X'
					}
				}
			case "traversal":
				m.Entries[0].Path = "../outside"
			case "absolute":
				m.Entries[0].Path = "/outside"
			case "duplicate":
				m.Entries = append(m.Entries, m.Entries[0])
			case "symlink parent":
				m.Entries = append(m.Entries, Entry{Path: "source/dangling/file", Type: "directory", Mode: 0755})
			case "hardlink":
				m.Entries[0].Type = "hardlink"
			case "unknown format":
				m.Format = "pocketlinx.environment.v999"
			case "header mismatch":
				entries[0].h.Mode = 0777
			case "extra member":
				entries = append(entries, member{tar.Header{Name: "extra", Mode: 0600, Typeflag: tar.TypeReg}, nil})
			case "missing member":
				entries = entries[:len(entries)-1]
			case "invalid definition":
				for i := range entries {
					if entries[i].h.Name == "environment.json" {
						entries[i].data = []byte(`{"version":99}`)
						entries[i].h.Size = int64(len(entries[i].data))
						m.Entries[i].Size = entries[i].h.Size
						hash := sha256.Sum256(entries[i].data)
						m.Entries[i].SHA256 = hex.EncodeToString(hash[:])
					}
				}
			case "link chain escape":
				// All links are lexically contained. Resolving the first link
				// shortens the second target, whose '..' then escapes source.
				m.Entries = append(m.Entries,
					Entry{Path: "source/long", Type: "directory", Mode: 0755},
					Entry{Path: "source/long/nested", Type: "directory", Mode: 0755},
					Entry{Path: "source/shallow", Type: "directory", Mode: 0755},
					Entry{Path: "source/long/nested/link", Type: "symlink", Mode: 0777, Link: "../../shallow"},
					Entry{Path: "source/x", Type: "symlink", Mode: 0777, Link: "long/nested/link/../../outside"})
			case "symlink cycle":
				m.Entries = append(m.Entries, Entry{Path: "source/a", Type: "symlink", Mode: 0777, Link: "b"}, Entry{Path: "source/b", Type: "symlink", Mode: 0777, Link: "a"})
			}
			bad := writeBundle(t, m, entries)
			dest := filepath.Join(t.TempDir(), "dest")
			if err := Restore(bad, dest, Limits{}); err == nil {
				t.Fatal("accepted malformed bundle")
			}
			assertUnpublished(t, dest)
		})
	}
}

func TestTruncationAndTrailingData(t *testing.T) {
	bundle := saved(t, fixture(t))
	data, err := os.ReadFile(bundle)
	must(t, err)
	for _, kind := range []string{"header", "payload", "one end block", "no end blocks", "appended"} {
		t.Run(kind, func(t *testing.T) {
			var bad []byte
			switch kind {
			case "header":
				bad = data[:50]
			case "payload":
				bad = data[:len(data)/2]
			case "one end block":
				bad = data[:len(data)-512]
			case "no end blocks":
				bad = data[:len(data)-1024]
			case "appended":
				bad = append(append([]byte{}, data...), 0)
			}
			p := filepath.Join(t.TempDir(), "bad")
			must(t, os.WriteFile(p, bad, 0600))
			dest := filepath.Join(t.TempDir(), "dest")
			if err := Restore(p, dest, Limits{}); err == nil {
				t.Fatal("accepted truncation/trailing data")
			}
			assertUnpublished(t, dest)
		})
	}
}

func TestLimitsAndLock(t *testing.T) {
	root := fixture(t)
	bundle := saved(t, root)
	for _, limits := range []Limits{{MaxBytes: 1}, {MaxEntries: 1}, {MaxManifestBytes: 1}, {MaxBytes: -1}} {
		dest := filepath.Join(t.TempDir(), "dest")
		if err := Restore(bundle, dest, limits); err == nil {
			t.Fatal("ignored limits")
		}
		assertUnpublished(t, dest)
	}
	lock, err := lockDirectory(root)
	must(t, err)
	defer lock.Close()
	if err := Save(root, filepath.Join(t.TempDir(), "out"), SaveOptions{Stopped: true}); err == nil {
		t.Fatal("ignored source lock")
	}
}

func TestReadOnlyDirectoryAndOwnership(t *testing.T) {
	root := fixture(t)
	p := filepath.Join(root, "source/readonly")
	must(t, os.Mkdir(p, 0755))
	must(t, os.WriteFile(filepath.Join(p, "data"), []byte("data"), 0444))
	must(t, os.Chmod(p, 0555))
	t.Cleanup(func() { _ = os.Chmod(p, 0755) })
	if os.Geteuid() == 0 {
		must(t, os.Chown(filepath.Join(p, "data"), 1234, 2345))
	}
	when := time.Unix(1700000000, 0)
	must(t, os.Chtimes(p, when, when))
	bundle := saved(t, root)
	dest := filepath.Join(t.TempDir(), "restored")
	must(t, Restore(bundle, dest, Limits{}))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dest, "source/readonly"), 0755) })
	info, err := os.Stat(filepath.Join(dest, "source/readonly"))
	must(t, err)
	if info.Mode().Perm() != 0555 || !info.ModTime().Equal(when) {
		t.Fatal("directory metadata lost")
	}
	if os.Geteuid() == 0 {
		info, err := os.Stat(filepath.Join(dest, "source/readonly/data"))
		must(t, err)
		u, g := ownership(info)
		if u != 1234 || g != 2345 {
			t.Fatal("ownership lost")
		}
	}
}

func TestExtendedAttributesRejected(t *testing.T) {
	root := fixture(t)
	err := syscall.Setxattr(filepath.Join(root, "source/hello.sh"), "user.plx-test", []byte("value"), 0)
	if err == syscall.ENOTSUP {
		t.Skip("filesystem does not support xattrs")
	}
	must(t, err)
	if err := Save(root, filepath.Join(t.TempDir(), "out"), SaveOptions{Stopped: true}); err == nil || !strings.Contains(err.Error(), "attributes") {
		t.Fatalf("expected xattr rejection, got %v", err)
	}
}

func TestLongPathsLinksAndDeterminism(t *testing.T) {
	root := fixture(t)
	name := strings.Repeat("a", 140) + " 日本語.txt"
	must(t, os.WriteFile(filepath.Join(root, "source", name), []byte("long name"), 0644))
	must(t, os.Mkdir(filepath.Join(root, "source/sub"), 0755))
	must(t, os.Symlink("../"+name, filepath.Join(root, "source/sub/link")))
	first := saved(t, root)
	second := saved(t, root)
	a, err := os.ReadFile(first)
	must(t, err)
	b, err := os.ReadFile(second)
	must(t, err)
	if !bytes.Equal(a, b) {
		t.Fatal("unchanged source produced different bundle")
	}
	dest := filepath.Join(t.TempDir(), "restored")
	must(t, Restore(first, dest, Limits{}))
	got, err := os.ReadFile(filepath.Join(dest, "source/sub/link"))
	must(t, err)
	if string(got) != "long name" {
		t.Fatal("long path/link data lost")
	}
}

func TestMountAndSymlinkBundleRejected(t *testing.T) {
	if err := rejectMounts("/proc"); err == nil {
		t.Fatal("accepted a live mounted filesystem")
	}
	bundle := saved(t, fixture(t))
	link := filepath.Join(t.TempDir(), "link")
	must(t, os.Symlink(bundle, link))
	dest := filepath.Join(t.TempDir(), "dest")
	if err := Restore(link, dest, Limits{}); err == nil {
		t.Fatal("accepted symlink bundle")
	}
	assertUnpublished(t, dest)
}

func TestInvalidOwnershipRejected(t *testing.T) {
	m, entries := readBundle(t, saved(t, fixture(t)))
	m.Entries[0].UID = ^uint32(0)
	p := writeBundle(t, m, entries)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := Restore(p, dest, Limits{}); err == nil {
		t.Fatal("accepted chown sentinel")
	}
	assertUnpublished(t, dest)
}

func TestOwnershipFailureRollsBack(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged process")
	}
	m, entries := readBundle(t, saved(t, fixture(t)))
	m.Entries[0].UID = 0
	entries[0].h.Uid = 0
	p := writeBundle(t, m, entries)
	dest := filepath.Join(t.TempDir(), "dest")
	if err := Restore(p, dest, Limits{}); err == nil {
		t.Fatal("silently changed ownership instead of failing")
	}
	assertUnpublished(t, dest)
}

func TestHardLinksMaterializedAsFiles(t *testing.T) {
	root := fixture(t)
	must(t, os.Link(filepath.Join(root, "source/hello.sh"), filepath.Join(root, "source/hardlink")))
	p := saved(t, root)
	dest := filepath.Join(t.TempDir(), "dest")
	must(t, Restore(p, dest, Limits{}))
	a, err := os.ReadFile(filepath.Join(dest, "source/hello.sh"))
	must(t, err)
	b, err := os.ReadFile(filepath.Join(dest, "source/hardlink"))
	must(t, err)
	if !bytes.Equal(a, b) {
		t.Fatal("hardlink contents lost")
	}
}

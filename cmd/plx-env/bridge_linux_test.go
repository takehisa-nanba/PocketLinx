//go:build linux && amd64

package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBridgeRejectsNonNativeAndSymlink(t *testing.T) {
	if err := nativeDirectory("/proc"); err == nil {
		t.Fatal("proc accepted")
	}
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := nativeDirectory(link); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestBridgeUploadFailures(t *testing.T) {
	base := t.TempDir()
	if err := nativeDirectory(base); err != nil {
		t.Skipf("requires native Linux filesystem: %v", err)
	}
	for _, kind := range []string{"bad hash", "bad archive"} {
		t.Run(kind, func(t *testing.T) {
			payload := []byte("not a bundle\x00\r\n")
			digest := fmt.Sprintf("%x", sha256.Sum256(payload))
			if kind == "bad hash" {
				digest = strings.Repeat("0", 64)
			}
			f, err := os.CreateTemp(base, "input")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			defer os.Remove(f.Name())
			f.Write(payload)
			f.Seek(0, 0)
			old := os.Stdin
			os.Stdin = f
			defer func() { os.Stdin = old }()
			target := filepath.Join(base, "restored")
			if err := runBridge([]string{"restore", target, digest}, io.Discard); err == nil {
				t.Fatal("bad upload accepted")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("failed upload published")
			}
			matches, _ := filepath.Glob(filepath.Join(base, ".plx-upload-*"))
			if len(matches) != 0 {
				t.Fatal("temporary upload leaked")
			}
		})
	}
}

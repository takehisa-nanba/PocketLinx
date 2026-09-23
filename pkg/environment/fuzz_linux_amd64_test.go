package environment

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func FuzzRestore(f *testing.F) {
	f.Add([]byte("not a bundle"))
	var seed bytes.Buffer
	w := tar.NewWriter(&seed)
	data, _ := json.Marshal(Manifest{Format: Format})
	_ = w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Typeflag: tar.TypeReg, Size: int64(len(data))})
	_, _ = w.Write(data)
	_ = w.Close()
	f.Add(seed.Bytes())
	// A valid seed reaches payload extraction and metadata handling.
	var valid bytes.Buffer
	definition := []byte(`{"version":1,"os":"linux","arch":"amd64","command":["/bin/sh"],"workdir":"/","uid":0,"gid":0}`)
	digest := sha256.Sum256(definition)
	m := Manifest{Format: Format, Entries: []Entry{
		{Path: "environment.json", Type: "file", Mode: 0600, Size: int64(len(definition)), SHA256: hex.EncodeToString(digest[:])},
		{Path: "rootfs", Type: "directory", Mode: 0755},
		{Path: "source", Type: "directory", Mode: 0755},
		{Path: "volumes", Type: "directory", Mode: 0755},
	}}
	data, _ = json.Marshal(m)
	w = tar.NewWriter(&valid)
	_ = w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Typeflag: tar.TypeReg, Size: int64(len(data))})
	_, _ = w.Write(data)
	for _, e := range m.Entries {
		_ = w.WriteHeader(header(e))
		if e.Type == "file" {
			_, _ = w.Write(definition)
		}
	}
	_ = w.Close()
	f.Add(valid.Bytes())
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 128<<10 {
			t.Skip()
		}
		base := t.TempDir()
		p := filepath.Join(base, "input")
		dest := filepath.Join(base, "output")
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		err := Restore(p, dest, Limits{MaxBytes: 64 << 10, MaxEntries: 64, MaxManifestBytes: 8 << 10})
		if err != nil {
			assertUnpublished(t, dest)
		}
	})
}

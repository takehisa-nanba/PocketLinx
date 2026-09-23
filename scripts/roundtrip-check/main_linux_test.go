//go:build linux && amd64

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSnapshotDetectsChangesAndDeletion(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"rootfs/bin", "source", "volumes"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	def := `{"version":1,"os":"linux","arch":"amd64","command":["/bin/tool"],"workdir":"/","uid":1000,"gid":1001}`
	os.WriteFile(filepath.Join(root, "environment.json"), []byte(def), 0644)
	file := filepath.Join(root, "rootfs/bin/tool")
	os.WriteFile(file, []byte("test"), 0755)
	os.Symlink("tool", filepath.Join(root, "rootfs/bin/link"))
	before, err := snapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "snapshot.json")
	b, _ := json.Marshal(before)
	os.WriteFile(evidence, b, 0600)
	if err := run([]string{"check", root, evidence}); err != nil {
		t.Fatal(err)
	}
	os.Chmod(file, 0644)
	after, err := snapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(before, after) {
		t.Fatal("mode change missed")
	}
	os.Chmod(file, 0755)
	os.WriteFile(file, []byte("changed"), 0755)
	if err := run([]string{"check", root, evidence}); err == nil {
		t.Fatal("content change missed")
	}
	os.Remove(filepath.Join(root, "rootfs/bin/link"))
	if err := run([]string{"check", root, evidence}); err == nil {
		t.Fatal("deletion missed")
	}
}

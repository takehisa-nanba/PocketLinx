package environment

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkSaveRestore(b *testing.B) {
	base := b.TempDir()
	src := filepath.Join(base, "source")
	for _, p := range []string{"rootfs", "source", "volumes"} {
		if err := os.MkdirAll(filepath.Join(src, p), 0755); err != nil {
			b.Fatal(err)
		}
	}
	definition := []byte(`{"version":1,"os":"linux","arch":"amd64","command":["/bin/sh"],"workdir":"/","uid":0,"gid":0}`)
	if err := os.WriteFile(filepath.Join(src, "environment.json"), definition, 0600); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "source/data"), make([]byte, 1<<20), 0644); err != nil {
		b.Fatal(err)
	}
	bundle := filepath.Join(base, "bundle")
	dst := filepath.Join(base, "restored")
	b.ReportAllocs()
	b.SetBytes(1 << 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Save(src, bundle, SaveOptions{Stopped: true}); err != nil {
			b.Fatal(err)
		}
		if err := Restore(bundle, dst, Limits{}); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		os.Remove(bundle)
		os.RemoveAll(dst)
		b.StartTimer()
	}
}

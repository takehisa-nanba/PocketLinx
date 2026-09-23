package envrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"PocketLinx/pkg/environment"
)

type backendFunc func(context.Context, Plan, Streams) error

func (f backendFunc) Run(c context.Context, p Plan, s Streams) error { return f(c, p, s) }
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func writeDefinition(t *testing.T, root string, d environment.Definition) {
	t.Helper()
	b, err := json.Marshal(d)
	must(t, err)
	must(t, os.WriteFile(filepath.Join(root, "environment.json"), b, 0600))
}

func fixture(t *testing.T) (string, environment.Definition) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "environment")
	d := environment.Definition{Version: 2, OS: "linux", Arch: "amd64", Command: []string{"/bin/tool", "a b", "", "'\";$()"}, Workdir: "/code", UID: 1234, GID: 2345, Env: map[string]string{"VALUE": "literal $()\n", "OTHER": "two"}, Source: &environment.SourcePlacement{Target: "/code"}, Volumes: []environment.VolumePlacement{{Name: "state", Target: "/persistent"}}}
	for _, p := range []string{"rootfs/bin", "rootfs/code", "rootfs/persistent", "source", "volumes/state"} {
		must(t, os.MkdirAll(filepath.Join(root, p), 0755))
	}
	must(t, os.WriteFile(filepath.Join(root, "rootfs/bin/tool"), []byte("test executable placeholder"), 0755))
	must(t, os.WriteFile(filepath.Join(root, "source/sentinel"), []byte("unchanged"), 0644))
	writeDefinition(t, root, d)
	return root, d
}

func TestDefinitionPlanAndSaveLock(t *testing.T) {
	root, d := fixture(t)
	called := false
	must(t, Run(context.Background(), root, backendFunc(func(_ context.Context, p Plan, _ Streams) error {
		called = true
		if !reflect.DeepEqual(d, p.Definition) {
			t.Fatal("definition altered")
		}
		if !reflect.DeepEqual(p.Environment, []string{"OTHER=two", "VALUE=literal $()\n"}) {
			t.Fatalf("wrong env: %q", p.Environment)
		}
		if p.Placements[0].Directory != filepath.Join(root, "source") || p.Placements[0].Target != d.Source.Target || p.Placements[1].Directory != filepath.Join(root, "volumes", d.Volumes[0].Name) {
			t.Fatal("wrong placement")
		}
		if err := environment.Save(root, filepath.Join(t.TempDir(), "blocked"), environment.SaveOptions{Stopped: true}); err == nil || !strings.Contains(err.Error(), "busy") {
			t.Fatalf("save not excluded during run: %v", err)
		}
		return nil
	}), Streams{}))
	if !called {
		t.Fatal("backend not invoked")
	}
	must(t, environment.Save(root, filepath.Join(t.TempDir(), "after"), environment.SaveOptions{Stopped: true}))
}

func TestInvalidDefinitionNeverReachesBackend(t *testing.T) {
	for _, kind := range []string{"missing command", "relative command", "missing workdir", "bad source", "bad volume", "duplicate target", "nested target", "duplicate volume", "missing volume", "symlink target", "nonempty target", "unknown field", "missing uid", "null gid", "legacy", "missing rootfs", "special file"} {
		t.Run(kind, func(t *testing.T) {
			root, d := fixture(t)
			switch kind {
			case "missing command":
				d.Command[0] = "/not-present"
			case "relative command":
				d.Command[0] = "tool"
			case "missing workdir":
				d.Workdir = "/not-present"
			case "bad source":
				d.Source.Target = "/code/../../outside"
			case "bad volume":
				d.Volumes[0].Target = "relative"
			case "duplicate target":
				d.Volumes[0].Target = d.Source.Target
			case "nested target":
				d.Volumes[0].Target = d.Source.Target + "/nested"
			case "duplicate volume":
				d.Volumes = append(d.Volumes, environment.VolumePlacement{Name: d.Volumes[0].Name, Target: "/another"})
			case "missing volume":
				must(t, os.Remove(filepath.Join(root, "volumes/state")))
			case "symlink target":
				must(t, os.Remove(filepath.Join(root, "rootfs/code")))
				must(t, os.Symlink("persistent", filepath.Join(root, "rootfs/code")))
			case "nonempty target":
				must(t, os.WriteFile(filepath.Join(root, "rootfs/code/do-not-hide"), []byte("keep"), 0600))
			case "legacy":
				d.Version = 1
				d.Source = nil
				d.Volumes = nil
			case "missing rootfs":
				must(t, os.RemoveAll(filepath.Join(root, "rootfs")))
			case "special file":
				must(t, os.Chmod(filepath.Join(root, "rootfs/bin/tool"), 0755|os.ModeSetuid))
			}
			writeDefinition(t, root, d)
			if kind == "unknown field" || kind == "missing uid" || kind == "null gid" {
				b, err := os.ReadFile(filepath.Join(root, "environment.json"))
				must(t, err)
				var m map[string]interface{}
				must(t, json.Unmarshal(b, &m))
				if kind == "unknown field" {
					m["hostPath"] = "/outside"
				}
				if kind == "missing uid" {
					delete(m, "uid")
				}
				if kind == "null gid" {
					m["gid"] = nil
				}
				b, err = json.Marshal(m)
				must(t, err)
				must(t, os.WriteFile(filepath.Join(root, "environment.json"), b, 0600))
			}
			called := false
			err := Run(context.Background(), root, backendFunc(func(context.Context, Plan, Streams) error { called = true; return nil }), Streams{})
			if err == nil || called {
				t.Fatalf("invalid environment executed: %v", err)
			}
			b, err := os.ReadFile(filepath.Join(root, "source/sentinel"))
			must(t, err)
			if string(b) != "unchanged" {
				t.Fatal("source changed on failure")
			}
		})
	}
}

func TestUnprivilegedBackendFailsExplicitly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root invocation")
	}
	root, _ := fixture(t)
	err := Run(context.Background(), root, ExperimentalChroot{Executable: "not-used"}, Streams{})
	if err == nil || !strings.Contains(err.Error(), "requires root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmptyEnvironmentIsNotInherited(t *testing.T) {
	root, d := fixture(t)
	d.Env = nil
	writeDefinition(t, root, d)
	must(t, Run(context.Background(), root, backendFunc(func(_ context.Context, p Plan, _ Streams) error {
		if p.Environment == nil || len(p.Environment) != 0 {
			t.Fatal("env must be an explicit empty slice")
		}
		return nil
	}), Streams{}))
}

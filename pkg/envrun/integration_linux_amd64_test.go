package envrun

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"PocketLinx/pkg/environment"
)

func integrationBinary(t *testing.T) string {
	t.Helper()
	if os.Getenv("PLX_ENV_INTEGRATION") != "1" {
		t.Skip("requires disposable Alpine with mount/chroot capabilities; run scripts/verify-environment.sh")
	}
	bin := os.Getenv("PLX_ENV_BINARY")
	if bin == "" {
		t.Fatal("PLX_ENV_BINARY is required")
	}
	return bin
}

func alpineFixture(t *testing.T, mutate func(*environment.Definition)) (string, environment.Definition) {
	t.Helper()
	sample := filepath.Join("..", "..", "examples", "environment")
	d, err := environment.LoadDefinition(filepath.Join(sample, "environment.json"))
	must(t, err)
	if mutate != nil {
		mutate(&d)
	}
	root := filepath.Join(t.TempDir(), "environment")
	for _, p := range []string{"rootfs/bin", "rootfs/lib", "source", "volumes"} {
		must(t, os.MkdirAll(filepath.Join(root, p), 0755))
	}
	copyFile(t, "/bin/busybox", filepath.Join(root, "rootfs/bin/busybox"), 0755)
	copyFile(t, "/lib/ld-musl-x86_64.so.1", filepath.Join(root, "rootfs/lib/ld-musl-x86_64.so.1"), 0755)
	must(t, os.Symlink("busybox", filepath.Join(root, "rootfs/bin/sh")))
	writeDefinition(t, root, d)
	placements := []string{d.Source.Target}
	for _, v := range d.Volumes {
		placements = append(placements, v.Target)
		must(t, os.MkdirAll(filepath.Join(root, "volumes", v.Name), 0755))
	}
	for _, target := range placements {
		must(t, os.MkdirAll(filepath.Join(root, "rootfs", filepath.FromSlash(strings.TrimPrefix(target, "/"))), 0755))
	}
	script := filepath.Join(root, "source", filepath.FromSlash(strings.TrimPrefix(d.Command[1], d.Source.Target+"/")))
	copyFile(t, filepath.Join(sample, "hello.sh"), script, 0755)
	must(t, os.WriteFile(dataFile(t, root, d), []byte("persistent-data\n"), 0644))
	for _, base := range []string{filepath.Join(root, "source"), filepath.Join(root, "volumes")} {
		must(t, filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			return os.Chown(p, int(d.UID), int(d.GID))
		}))
	}
	return root, d
}

func copyFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	b, err := os.ReadFile(src)
	must(t, err)
	must(t, os.WriteFile(dst, b, mode))
}
func dataFile(t *testing.T, root string, d environment.Definition) string {
	t.Helper()
	for _, v := range d.Volumes {
		if strings.HasPrefix(d.Env["DATA_FILE"], v.Target+"/") {
			return filepath.Join(root, "volumes", v.Name, filepath.FromSlash(strings.TrimPrefix(d.Env["DATA_FILE"], v.Target+"/")))
		}
	}
	t.Fatal("sample DATA_FILE is not inside a declared volume")
	return ""
}
func invoke(t *testing.T, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("CLI %v failed: %v\n%s", args, err, stderr.String())
	}
	return out.String()
}
func sampleOutput(d environment.Definition, value string) string {
	return fmt.Sprintf("%s|%s|%s|uid=%d|gid=%d\n", d.Env["GREETING"], d.Workdir, value, d.UID, d.GID)
}

func TestIntegrationDefinitionRoundTrip(t *testing.T) {
	bin := integrationBinary(t)
	root, d := alpineFixture(t, nil)
	before := invoke(t, bin, "run", root)
	if before != sampleOutput(d, "persistent-data") {
		t.Fatalf("definition not reproduced: %q", before)
	}
	base := t.TempDir()
	first := filepath.Join(base, "first.plxenv")
	restored := filepath.Join(base, "restored")
	invoke(t, bin, "save", "--stopped", root, first)
	invoke(t, bin, "restore", first, restored)
	if after := invoke(t, bin, "run", restored); after != before {
		t.Fatalf("restored output differs: %q", after)
	}
	script := filepath.Join(restored, "source", filepath.FromSlash(strings.TrimPrefix(d.Command[1], d.Source.Target+"/")))
	f, err := os.OpenFile(script, os.O_APPEND|os.O_WRONLY, 0)
	must(t, err)
	// These edits are program behavior, not runtime configuration. DATA_FILE
	// comes from Definition, and writes must reach the original mapped volume.
	_, err = f.WriteString("\nprintf 'edited\\n'\nprintf 'runtime-write\\n' > \"$DATA_FILE\"\nprintf 'source-write\\n' > generated.txt\n")
	must(t, err)
	must(t, f.Close())
	must(t, os.WriteFile(dataFile(t, restored, d), []byte("updated-data\n"), 0644))
	if got := invoke(t, bin, "run", restored); got != sampleOutput(d, "updated-data")+"edited\n" {
		t.Fatalf("edited execution: %q", got)
	}
	content, err := os.ReadFile(dataFile(t, restored, d))
	must(t, err)
	if string(content) != "runtime-write\n" {
		t.Fatal("runtime volume write not persistent")
	}
	content, err = os.ReadFile(filepath.Join(restored, "source/generated.txt"))
	must(t, err)
	if string(content) != "source-write\n" {
		t.Fatal("runtime source write not persistent")
	}
	second := filepath.Join(base, "second.plxenv")
	back := filepath.Join(base, "back")
	invoke(t, bin, "save", "--stopped", restored, second)
	invoke(t, bin, "restore", second, back)
	if got := invoke(t, bin, "run", back); got != sampleOutput(d, "runtime-write")+"edited\n" {
		t.Fatalf("resaved definition execution: %q", got)
	}
	content, err = os.ReadFile(dataFile(t, root, d))
	must(t, err)
	if string(content) != "persistent-data\n" {
		t.Fatal("original changed")
	}
	for _, target := range []string{d.Source.Target, d.Volumes[0].Target} {
		entries, err := os.ReadDir(filepath.Join(restored, "rootfs", strings.TrimPrefix(target, "/")))
		must(t, err)
		if len(entries) != 0 {
			t.Fatal("mount leaked into caller namespace")
		}
	}
}

func TestIntegrationArgumentsEnvironmentAndRelocation(t *testing.T) {
	bin := integrationBinary(t)
	canary := filepath.Join(t.TempDir(), "must-not-exist")
	root, d := alpineFixture(t, func(d *environment.Definition) {
		d.Source.Target = "/renamed source"
		d.Workdir = d.Source.Target
		d.Command[1] = path.Join(d.Source.Target, path.Base(d.Command[1]))
		d.Volumes[0].Target = "/renamed state"
		d.Env["DATA_FILE"] = path.Join(d.Volumes[0].Target, path.Base(d.Env["DATA_FILE"]))
		d.Env["GREETING"] = "literal $(touch " + canary + ") ' \""
		d.Command = append(d.Command, "arg with spaces", "", "'\";$()")
	})
	script := filepath.Join(root, "source", path.Base(d.Command[1]))
	f, err := os.OpenFile(script, os.O_WRONLY|os.O_APPEND, 0)
	must(t, err)
	_, err = f.WriteString("\n[ -z \"${POCKETLINX_HOST_ONLY+x}\" ] || exit 91\nprintf '[%s][%s][%s]\\n' \"$1\" \"$2\" \"$3\"\n/bin/busybox id -G\n")
	must(t, err)
	must(t, f.Close())
	t.Setenv("POCKETLINX_HOST_ONLY", "must not inherit")
	expected := sampleOutput(d, "persistent-data") + fmt.Sprintf("[%s][%s][%s]\n%d\n", d.Command[2], d.Command[3], d.Command[4], d.GID)
	if got := invoke(t, bin, "run", root); got != expected {
		t.Fatalf("lost argv/env/credentials/placements\nwant %q\ngot  %q", expected, got)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatal("shell expansion modified host")
	}
}

func TestIntegrationCredentialDenied(t *testing.T) {
	if os.Getenv("PLX_ENV_EXPECT_CREDENTIAL_DENIED") != "1" {
		t.Skip("requires CAP_SETUID removed from the disposable container")
	}
	bin := integrationBinary(t)
	root, _ := alpineFixture(t, nil)
	cmd := exec.Command(bin, "run", root)
	out, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(out, []byte("operation not permitted")) {
		t.Fatalf("credentials were not rejected: %v %s", err, out)
	}
}

func TestIntegrationReadOnlyRootfsAndExitStatus(t *testing.T) {
	bin := integrationBinary(t)
	root, d := alpineFixture(t, func(d *environment.Definition) { d.UID = 0; d.GID = 0 })
	script := filepath.Join(root, "source", path.Base(d.Command[1]))
	must(t, os.WriteFile(script, []byte("#!/bin/sh\nif (printf forbidden > /must-not-exist); then exit 92; fi\nexit 37\n"), 0755))
	cmd := exec.Command(bin, "run", root)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 37 || !bytes.Contains(out, []byte("Read-only file system")) {
		t.Fatalf("rootfs protection / exit status: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "rootfs/must-not-exist")); !os.IsNotExist(err) {
		t.Fatal("rootfs changed")
	}
	// Failure must release the run lock and namespace mounts, allowing save.
	invoke(t, bin, "save", "--stopped", root, filepath.Join(t.TempDir(), "after-failure.plxenv"))
}

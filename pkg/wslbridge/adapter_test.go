package wslbridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTransferAndArguments(t *testing.T) {
	payload := []byte("binary\x00\xff\r\n\n$()'\"")
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	dir := "/opt/sample with spaces;$(touch nope)"
	a := Adapter{Distro: "Test-Distro", User: "root", LinuxCLI: "/opt/cli with spaces"}
	a.Invoke = func(_ context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
		if reflect.DeepEqual(args, []string{"--list", "--verbose"}) {
			io.WriteString(out, "  NAME STATE VERSION\n* Test-Distro Running 2\n")
			return nil
		}
		prefix := []string{"--distribution", "Test-Distro", "--user", "root", "--exec", "/opt/cli with spaces", "bridge"}
		if len(args) < 8 || !reflect.DeepEqual(args[:7], prefix) {
			t.Fatalf("argument corruption: %q", args)
		}
		switch args[7] {
		case "check":
			io.WriteString(out, "plx-env-wsl-bridge-v1 linux/amd64\n")
		case "restore":
			if args[8] != dir || args[9] != digest {
				t.Fatal(args)
			}
			b, _ := io.ReadAll(in)
			if !bytes.Equal(b, payload) {
				t.Fatal("binary changed")
			}
			io.WriteString(out, digest+"\n")
		case "save":
			if args[8] != dir {
				t.Fatal(args)
			}
			for _, b := range append([]byte(digest+"\n"), payload...) {
				if _, err := out.Write([]byte{b}); err != nil {
					return err
				}
			}
		case "run":
			if args[8] != dir {
				t.Fatal(args)
			}
			io.WriteString(out, "ran")
		default:
			t.Fatal(args)
		}
		return nil
	}
	ctx := context.Background()
	base := t.TempDir()
	input := filepath.Join(base, "input quote ' space.plxenv")
	os.WriteFile(input, payload, 0600)
	if err := a.Restore(ctx, input, dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(base, "output.plxenv")
	if err := a.Save(ctx, dir, output, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(output)
	if !bytes.Equal(got, payload) {
		t.Fatal("download changed")
	}
	if err := a.Save(ctx, dir, output, io.Discard); err == nil {
		t.Fatal("overwrote existing package")
	}
	var b bytes.Buffer
	if err := a.Run(ctx, dir, nil, &b, io.Discard); err != nil || b.String() != "ran" {
		t.Fatal(err)
	}
}
func TestBadTransfersNeverPublish(t *testing.T) {
	for _, stream := range []string{"short", strings.Repeat("0", 64) + "\npayload", "bad header"} {
		t.Run(stream[:3], func(t *testing.T) {
			a := Adapter{Distro: "Test", User: "root", LinuxCLI: "/bin/cli"}
			a.Invoke = func(_ context.Context, args []string, _ io.Reader, out, _ io.Writer) error {
				if args[0] == "--list" {
					io.WriteString(out, "Test Running 2\n")
				} else if args[len(args)-1] == "check" {
					io.WriteString(out, "plx-env-wsl-bridge-v1 linux/amd64\n")
				} else {
					_, err := io.WriteString(out, stream)
					return err
				}
				return nil
			}
			target := filepath.Join(t.TempDir(), "out")
			if err := a.Save(context.Background(), "/opt/env", target, io.Discard); err == nil {
				t.Fatal("accepted bad stream")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("published failure")
			}
		})
	}
}
func TestSelectionErrors(t *testing.T) {
	for _, distro := range []string{"docker-desktop", "docker-desktop-data", "bad;name", "Missing", "WSL1"} {
		a := Adapter{Distro: distro, User: "root", LinuxCLI: "/bin/cli", Invoke: func(_ context.Context, _ []string, _ io.Reader, out, _ io.Writer) error {
			io.WriteString(out, "WSL1 Stopped 1\n")
			return nil
		}}
		if err := a.Check(context.Background(), io.Discard); err == nil {
			t.Fatal(distro)
		}
	}
	raw := []byte{}
	for _, c := range "* Test Running 2\r\n" {
		raw = append(raw, byte(c), 0)
	}
	if decodeListing(raw) != "* Test Running 2\r\n" {
		t.Fatal("UTF16 listing")
	}
}

func TestUnavailableWSLPreservesError(t *testing.T) {
	cause := errors.New("WSL executable unavailable")
	a := Adapter{Distro: "Test", User: "root", LinuxCLI: "/bin/cli", Invoke: func(context.Context, []string, io.Reader, io.Writer, io.Writer) error { return cause }}
	if err := a.Check(context.Background(), io.Discard); !errors.Is(err, cause) {
		t.Fatalf("lost process error: %v", err)
	}
}

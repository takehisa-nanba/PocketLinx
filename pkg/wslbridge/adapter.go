// Package wslbridge transports bundles and invokes the existing Linux CLI.
// It does not implement environment storage or execution.
package wslbridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

const MaxTransferBytes int64 = 9 << 30

type Runner func(context.Context, []string, io.Reader, io.Writer, io.Writer) error
type Adapter struct {
	Distro, LinuxCLI, User string
	Invoke                 Runner
}

// SystemRunner is Windows-only and resolves the system WSL executable.
func SystemRunner(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	exe, err := systemWSL()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, stderr
	return cmd.Run()
}

func LinuxPath(p string) bool {
	return strings.HasPrefix(p, "/") && p != "/" && path.Clean(p) == p && !strings.ContainsAny(p, "\\\x00\r\n")
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func (a Adapter) Check(ctx context.Context, stderr io.Writer) error {
	if !identifier.MatchString(a.Distro) || strings.HasPrefix(strings.ToLower(a.Distro), "docker-desktop") {
		return fmt.Errorf("select an independent WSL distro (Docker Desktop is prohibited)")
	}
	if !identifier.MatchString(a.User) || !LinuxPath(a.LinuxCLI) {
		return fmt.Errorf("explicit Linux user and absolute Linux CLI path required")
	}
	if a.Invoke == nil {
		return fmt.Errorf("WSL runner required")
	}
	var output bytes.Buffer
	if err := a.Invoke(ctx, []string{"--list", "--verbose"}, nil, &output, stderr); err != nil {
		return fmt.Errorf("WSL unavailable: %w", err)
	}
	listing := decodeListing(output.Bytes())
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*")))
		if len(fields) >= 3 && fields[0] == a.Distro {
			if fields[len(fields)-1] != "2" {
				return fmt.Errorf("selected distribution must use WSL2")
			}
			output.Reset()
			if err := a.call(ctx, []string{"bridge", "check"}, nil, &output, stderr); err != nil {
				return err
			}
			if strings.TrimSpace(output.String()) != "plx-env-wsl-bridge-v1 linux/amd64" {
				return fmt.Errorf("incompatible Linux bridge CLI")
			}
			return nil
		}
	}
	return fmt.Errorf("WSL distribution %q not found", a.Distro)
}
func decodeListing(b []byte) string {
	if bytes.Contains(b, []byte{0}) {
		units := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			units = append(units, binary.LittleEndian.Uint16(b[i:]))
		}
		return strings.TrimPrefix(string(utf16.Decode(units)), "\ufeff")
	}
	return string(b)
}
func (a Adapter) call(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	argv := []string{"--distribution", a.Distro, "--user", a.User, "--exec", a.LinuxCLI}
	return a.Invoke(ctx, append(argv, args...), in, out, stderr)
}
func (a Adapter) Restore(ctx context.Context, bundle, destination string, stderr io.Writer) error {
	if !LinuxPath(destination) {
		return fmt.Errorf("invalid Linux destination")
	}
	if err := a.Check(ctx, stderr); err != nil {
		return err
	}
	f, err := os.Open(bundle)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxTransferBytes {
		return fmt.Errorf("invalid bundle file or transfer limit exceeded")
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var result bytes.Buffer
	if err = a.call(ctx, []string{"bridge", "restore", destination, digest}, f, &result, stderr); err != nil {
		return fmt.Errorf("WSL restore: %w", err)
	}
	if strings.TrimSpace(result.String()) != digest {
		return fmt.Errorf("remote SHA-256 acknowledgement mismatch")
	}
	fmt.Fprintf(stderr, "SHA-256 verified Windows -> WSL: %s\n", digest)
	return nil
}
func (a Adapter) Run(ctx context.Context, directory string, in io.Reader, out, stderr io.Writer) error {
	if !LinuxPath(directory) {
		return fmt.Errorf("invalid Linux directory")
	}
	if err := a.Check(ctx, stderr); err != nil {
		return err
	}
	return a.call(ctx, []string{"bridge", "run", directory}, in, out, stderr)
}
func (a Adapter) Save(ctx context.Context, directory, bundle string, stderr io.Writer) error {
	if !LinuxPath(directory) {
		return fmt.Errorf("invalid Linux directory")
	}
	if err := a.Check(ctx, stderr); err != nil {
		return err
	}
	if _, err := os.Lstat(bundle); !os.IsNotExist(err) {
		return fmt.Errorf("destination exists or is inaccessible")
	}
	parent := filepath.Dir(bundle)
	f, err := os.CreateTemp(parent, ".plx-transfer-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	// stdout protocol is exactly 64 lowercase hex bytes, LF, then raw bundle.
	receiver := &receiver{file: f, hash: sha256.New()}
	if err = a.call(ctx, []string{"bridge", "save", directory}, nil, receiver, stderr); err != nil {
		return fmt.Errorf("WSL save: %w", err)
	}
	if err = receiver.finish(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Hard-link publication is atomic and never replaces an existing destination.
	if err = os.Link(f.Name(), bundle); err != nil {
		return fmt.Errorf("publish without overwrite (hard links required): %w", err)
	}
	fmt.Fprintf(stderr, "SHA-256 verified WSL -> Windows: %s\n", string(receiver.header[:64]))
	return nil
}

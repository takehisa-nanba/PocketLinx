package wsl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client handles interaction with the WSL backend
type Client struct {
	DistroName string
}

// NewClient creates a new WSL client
func NewClient(distroName string) *Client {
	return &Client{DistroName: distroName}
}

// RunCommand executes a command inside the WSL distro
// direct: if true, runs "wsl -d distro -- cmd..."
// if false, runs "wsl cmd..." (e.g. for --import)
func (c *Client) Run(args ...string) error {
	cmd := exec.Command("wsl.exe", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunDistroCommand executes a command INSIDE the specific distro
func (c *Client) RunDistroCommand(args ...string) error {
	wslArgs := append([]string{"-d", c.DistroName, "--"}, args...)
	return c.Run(wslArgs...)
}

// RunDistroCommandWithInput executes a command with stdin input (useful for scripts)
func (c *Client) RunDistroCommandWithInput(input string, args ...string) error {
	wslArgs := append([]string{"-d", c.DistroName, "--"}, args...)
	cmd := exec.Command("wsl.exe", wslArgs...)
	// Sanitize input (CRLF -> LF) to prevent syntax errors in Linux
	input = strings.ReplaceAll(input, "\r\n", "\n")
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunDistroCommandOutput executes a command INSIDE the specific distro and returns output
func (c *Client) RunDistroCommandOutput(args ...string) (string, error) {
	wslArgs := append([]string{"-d", c.DistroName, "--"}, args...)
	cmd := exec.Command("wsl.exe", wslArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// WindowsToWslPath converts a Windows path to a WSL path
func WindowsToWslPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// 1. Clean and get absolute path
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// 2. Handle UNC Paths (\\host\share\...)
	if strings.HasPrefix(abs, "\\\\") {
		// UNC paths in WSL2 are often accessed via /mnt/wsl/ (custom) or handled as SMB mounts.
		// Standard wslpath converts \\host\share to /mnt/wsl/host/share (approx).
		// For PocketLinx, we'll convert to a generic /mnt/wsl/ style to stay consistent.
		parts := strings.Split(strings.TrimPrefix(abs, "\\\\"), "\\")
		return "/mnt/wsl/" + strings.Join(parts, "/"), nil
	}

	// 3. Handle Drive Letters (C:\...)
	if len(abs) >= 3 && abs[1] == ':' && abs[2] == '\\' {
		drive := strings.ToLower(string(abs[0]))
		rest := filepath.ToSlash(abs[3:])
		return fmt.Sprintf("/mnt/%s/%s", drive, rest), nil
	}

	// 4. Fallback (already looks like a Linux path or relative without drive)
	return filepath.ToSlash(abs), nil
}

// StartDistroCommand starts a command inside the specific distro but does not wait for completion
func (c *Client) StartDistroCommand(args ...string) (*exec.Cmd, error) {
	wslArgs := append([]string{"-d", c.DistroName, "--"}, args...)
	cmd := exec.Command("wsl.exe", wslArgs...)
	// Stderr should usually be piped or shared
	cmd.Stderr = os.Stderr
	// No stdout by default to allow custom piping/monitoring
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// PrepareDistroCommand creates a command inside the specific distro but does not start it
func (c *Client) PrepareDistroCommand(args ...string) *exec.Cmd {
	wslArgs := append([]string{"-d", c.DistroName, "--"}, args...)
	cmd := exec.Command("wsl.exe", wslArgs...)
	cmd.Stderr = os.Stderr
	return cmd
}

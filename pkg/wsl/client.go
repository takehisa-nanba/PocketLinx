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
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WindowsToWslPath converts a Windows path (e.g. C:\Users) to a WSL path (/mnt/c/Users)
func WindowsToWslPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// C:\Users -> /mnt/c/Users
	drive := abs[0]
	driveLower := string(drive + 32) // Naive lowercase
	rest := abs[3:]
	rest = filepath.ToSlash(rest)

	return fmt.Sprintf("/mnt/%s/%s", driveLower, rest), nil
}

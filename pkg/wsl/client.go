package wsl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// Session handles a persistent connection to a WSL distro (v1.1.4)
type Session struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner
}

// NewSession creates and starts a persistent WSL session
func (c *Client) NewSession() (*Session, error) {
	// Start sh in the distro as a persistent process
	wslArgs := []string{"-d", c.DistroName, "-u", "root", "--", "sh"}
	cmd := exec.Command("wsl.exe", wslArgs...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &Session{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		scanner: bufio.NewScanner(stdout),
	}, nil
}

// Execute runs a command and waits for the specific "DONE" signal with exit code
func (s *Session) Execute(command string) (string, error) {
	sentinel := "__PLX_DONE__"
	// Wrap in braces to handle multi-line scripts as a single block for exit code accuracy
	input := fmt.Sprintf("{\n%s\n}\necho \"%s $?\"\n", command, sentinel)

	if os.Getenv("PLX_VERBOSE") != "" {
		fmt.Printf("[DEBUG] Session Exec (length: %d)\n", len(input))
	}

	if _, err := io.WriteString(s.stdin, input); err != nil {
		return "", fmt.Errorf("failed to write to session: %w", err)
	}

	// Capture output while checking for sentinel
	var output []string
	for s.scanner.Scan() {
		line := s.scanner.Text()

		// If VERBOSE is on, echo to terminal immediately (v1.1.5)
		if os.Getenv("PLX_VERBOSE") != "" {
			fmt.Println(line)
		}

		if strings.HasPrefix(line, sentinel) {
			// Parse exit code: "__PLX_DONE__ 0"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				code, err := strconv.Atoi(parts[1])
				if err == nil && code != 0 {
					return strings.Join(output, "\n"), fmt.Errorf("command failed with exit code %d: %s", code, strings.Join(output, "\n"))
				}
			}
			return strings.Join(output, "\n"), nil
		}
		output = append(output, line)
	}

	if err := s.scanner.Err(); err != nil {
		return strings.Join(output, "\n"), err
	}

	return strings.Join(output, "\n"), io.EOF
}

// Close terminates the session
func (s *Session) Close() error {
	if s.stdin != nil {
		io.WriteString(s.stdin, "exit\n")
		s.stdin.Close()
	}
	return s.cmd.Wait()
}

// Become transforms the session into the final container process (v1.1.5)
func (s *Session) Become(args []string) error {
	command := strings.Join(args, " ")
	// Send 'exec' to replace the sh process with the actual container process
	input := fmt.Sprintf("exec %s\n", command)

	if os.Getenv("PLX_VERBOSE") != "" {
		fmt.Printf("[DEBUG] Session Becoming: %s\n", input)
	}

	if _, err := io.WriteString(s.stdin, input); err != nil {
		return fmt.Errorf("failed to send exec to session: %w", err)
	}

	// Now that it's 'exec'-ed, we hand over everything.
	// We need to copy os.Stdin to s.stdin and s.stdout to os.Stdout concurrently
	// since we can't re-assign pipes of a running cmd.

	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(s.stdin, os.Stdin)
		errChan <- err
	}()

	go func() {
		// Continue scanning or plain copying?
		// Since sentinel logic is no longer used, plain copy is better
		_, err := io.Copy(os.Stdout, s.stdout)
		errChan <- err
	}()

	// Wait for the WSL process to exit
	return s.cmd.Wait()
}

// Run satisfies the container.CommandRunner interface (v1.1.4)
func (s *Session) Run(command string) (string, error) {
	return s.Execute(command)
}

// WaitUntilReady waits for the distro to be responsive (v1.1.3)
func (c *Client) WaitUntilReady(maxRetries int, interval time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		// Simple command to check if distro is responsive
		// Use RunDistroCommandOutput to avoid inheriting sterr/stdout for quiet check
		_, err := c.RunDistroCommandOutput("test", "-d", "/")
		if err == nil {
			return nil
		}
		if os.Getenv("PLX_VERBOSE") != "" {
			fmt.Printf("[DEBUG] WSL Distro %s not ready (err: %v), retrying in %v... (%d/%d)\n", c.DistroName, err, interval, i+1, maxRetries)
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("WSL distro %s failed to become ready after %d retries", c.DistroName, maxRetries)
}

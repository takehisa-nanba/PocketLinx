//go:build linux

package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type LinuxRuntimeService struct {
	rootDir string
}

func NewLinuxRuntimeService(rootDir string) *LinuxRuntimeService {
	return &LinuxRuntimeService{rootDir: rootDir}
}

func (s *LinuxRuntimeService) Run(opts RunOptions) error {
	containerId := fmt.Sprintf("c-%x", time.Now().UnixNano())

	containerDir := filepath.Join(s.rootDir, "containers", containerId)
	rootfsDir := filepath.Join(containerDir, "rootfs")

	image := opts.Image
	if image == "" {
		image = "alpine"
	}

	imageFile := filepath.Join(s.rootDir, "images", image+".tar.gz")
	if _, err := os.Stat(imageFile); os.IsNotExist(err) {
		return fmt.Errorf("image '%s' not found.", image)
	}

	// 1. Provisioning
	// Create dirs
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return fmt.Errorf("failed to create container dir: %w", err)
	}

	// Extract tar
	fmt.Printf("Extracting %s to %s...\n", imageFile, rootfsDir)
	tarCmd := exec.Command("tar", "-xf", imageFile, "-C", rootfsDir)
	tarCmd.Stdout = os.Stdout
	tarCmd.Stderr = os.Stderr
	if err := tarCmd.Run(); err != nil {
		return fmt.Errorf("failed to extract rootfs: %w", err)
	}

	// 2. Metadata
	meta := Container{
		ID:      containerId,
		Command: strings.Join(opts.Args, " "),
		Created: time.Now(),
		Status:  "Running",
	}
	metaJSON, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(containerDir, "config.json"), metaJSON, 0644); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}

	// 3. Mounts
	mountsStr := "none"
	if len(opts.Mounts) > 0 {
		var mParts []string
		for _, m := range opts.Mounts {
			absSrc, _ := filepath.Abs(m.Source)
			mParts = append(mParts, fmt.Sprintf("%s:%s", absSrc, m.Target))
		}
		mountsStr = strings.Join(mParts, ",")
	}

	// Service Discovery (Hosts)
	hostsPath := filepath.Join(rootfsDir, "etc", "hosts")
	_ = os.MkdirAll(filepath.Dir(hostsPath), 0755)

	// Default hosts
	hostsContent := "127.0.0.1 localhost\n::1 localhost ip6-localhost ip6-loopback\nfe00::0 ip6-localnet\nff00::0 ip6-mcastprefix\nff02::1 ip6-allnodes\nff02::2 ip6-allrouters\n"
	if opts.Name != "" {
		hostsContent += fmt.Sprintf("127.0.0.1 %s\n", opts.Name)
	}

	// Scan siblings
	if containers, err := s.List(); err == nil {
		for _, c := range containers {
			if c.Status == "Running" && c.Name != "" {
				hostsContent += fmt.Sprintf("127.0.0.1 %s\n", c.Name)
			}
		}
	}
	// Explicit ExtraHosts
	for _, h := range opts.ExtraHosts {
		parts := strings.Split(h, ":")
		if len(parts) == 2 {
			hostsContent += fmt.Sprintf("%s %s\n", parts[1], parts[0])
		}
	}

	if err := os.WriteFile(hostsPath, []byte(hostsContent), 0644); err != nil {
		fmt.Printf("Warning: Failed to write hosts file: %v\n", err)
	}

	// 4. Execution
	// unshare setup
	cmdArgs := []string{"--mount", "--pid", "--fork", "--uts", "--propagation", "unchanged"}

	// Add shim and args
	// We call plx-shim which sets up pivot_root, PATH, and user logic
	workdir := opts.Workdir
	if workdir == "" {
		workdir = "none"
	}
	user := opts.User
	if user == "" {
		user = "none"
	}

	cmdArgs = append(cmdArgs, "/usr/local/bin/plx-shim", rootfsDir, mountsStr, workdir, user, "none")
	cmdArgs = append(cmdArgs, opts.Args...)

	runCmd := exec.Command("unshare", cmdArgs...)

	// Env & IO
	runCmd.Stdin = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Env = os.Environ() // Pass current env

	if opts.Interactive {
		// handle tty checks? For now just inherit
	}

	fmt.Printf("Running container %s (Linux)...\n", containerId)
	err := runCmd.Run()

	// Cleanup / Update status
	meta.Status = "Exited"
	metaJSON, _ = json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(containerDir, "config.json"), metaJSON, 0644)

	return err
}

func (s *LinuxRuntimeService) List() ([]Container, error) {
	containersDir := filepath.Join(s.rootDir, "containers")
	entries, err := os.ReadDir(containersDir)
	if err != nil {
		return nil, nil
	}

	var containers []Container
	for _, e := range entries {
		if e.IsDir() {
			configPath := filepath.Join(containersDir, e.Name(), "config.json")
			data, err := os.ReadFile(configPath)
			if err == nil {
				var c Container
				if err := json.Unmarshal(data, &c); err == nil {
					containers = append(containers, c)
				}
			}
		}
	}
	return containers, nil
}

func (s *LinuxRuntimeService) Start(id string) error {
	fmt.Println("Start not fully implemented for Linux Native yet.")
	return nil
}

func (s *LinuxRuntimeService) Stop(id string) error {
	fmt.Println("Stop not fully implemented for Linux Native yet (requires PID tracking).")
	return nil
}

func (s *LinuxRuntimeService) Logs(id string) (string, error) {
	return "", fmt.Errorf("logs are not captured in native mode yet (stdout is attached)")
}

func (s *LinuxRuntimeService) Remove(id string) error {
	containerDir := filepath.Join(s.rootDir, "containers", id)
	return os.RemoveAll(containerDir)
}

func (s *LinuxRuntimeService) GetIP(id string) (string, error) {
	return "127.0.0.1", nil
}
func (s *LinuxRuntimeService) Update(id string, opts RunOptions) error {
	return fmt.Errorf("update not implemented")
}

func (s *LinuxRuntimeService) Exec(id string, cmd []string, interactive bool) error {
	return fmt.Errorf("exec not implemented for native linux yet")
}

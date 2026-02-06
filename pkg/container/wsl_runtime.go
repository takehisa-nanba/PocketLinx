package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"PocketLinx/pkg/shim"
	"PocketLinx/pkg/wsl"
)

// WSLRuntimeService implements RuntimeService
type WSLRuntimeService struct {
	wslClient *wsl.Client
	volume    VolumeService // Dependency on VolumeService for mounting? Maybe needed?
	// Actually logic in Run uses GetWslVolumesDir which is public now in wsl_volume (if I exported it? yes I did export it in wsl_volume.go, but it's in same package so it's fine)
	// But wait, Run also creates volumes if they don't exist?
	// The original code:
	// if err := b.wslClient.RunDistroCommand("mkdir", "-p", srcWsl);
	// It basically assumes it can run mkdir. It doesn't use CreateVolume explicitly but raw mkdir.
	// So we can keep it as is, or better, use VolumeService?
	// For now, keep it raw to minimize changes.
}

func NewWSLRuntimeService(client *wsl.Client) *WSLRuntimeService {
	return &WSLRuntimeService{wslClient: client}
}

func (s *WSLRuntimeService) Run(opts RunOptions) error {
	containerId := fmt.Sprintf("c-%d", os.Getpid())
	fmt.Printf("Running %v in container %s (WSL2)...\n", opts.Args, containerId)

	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", containerId)
	rootfsDir := path.Join(containerDir, "rootfs")

	// Metadata command string
	userCmd := strings.Join(opts.Args, " ")

	image := opts.Image
	if image == "" {
		image = "alpine"
	}

	wslImgPath := path.Join(GetWslImagesDir(), image+".tar.gz")

	// Check existence in WSL
	if err := s.wslClient.RunDistroCommand("test", "-f", wslImgPath); err != nil {
		return fmt.Errorf("image '%s' not found. Please run 'plx pull %s' first", image, image)
	}

	wslRootfsPath := wslImgPath
	var err error

	// 1. Provisioning
	setupCmd := fmt.Sprintf("mkdir -p %s && mkdir -p %s && tar -xf %s -C %s", containerDir, rootfsDir, wslRootfsPath, rootfsDir)
	if err := s.wslClient.RunDistroCommand("sh", "-c", setupCmd); err != nil {
		return fmt.Errorf("provisioning failed (path: %s): %w", containerDir, err)
	}

	// Update Shim
	if err := s.wslClient.RunDistroCommandWithInput(shim.Content, "sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim"); err != nil {
		fmt.Printf("Warning: Failed to update shim: %v\n", err)
	}

	// 2. Process Mounts
	mountsStr := "none"
	if len(opts.Mounts) > 0 {
		var mParts []string
		for _, m := range opts.Mounts {
			var srcWsl string

			isPath := strings.Contains(m.Source, "/") || strings.Contains(m.Source, "\\") || strings.Contains(m.Source, ".")

			if !isPath {
				// Named Volume
				volName := m.Source
				srcWsl = path.Join(GetWslVolumesDir(), volName)
				// Ensure volume exists (Auto-create on Run)
				if err := s.wslClient.RunDistroCommand("mkdir", "-p", srcWsl); err != nil {
					fmt.Printf("Warning: Failed to create/ensure volume %s: %v\n", volName, err)
					continue
				}
			} else {
				// Bind Mount
				absSource, _ := filepath.Abs(m.Source)
				var err error
				srcWsl, err = wsl.WindowsToWslPath(absSource)
				if err != nil {
					fmt.Printf("Warning: Failed to convert mount path %s: %v\n", m.Source, err)
					continue
				}
			}

			mParts = append(mParts, fmt.Sprintf("%s:%s", srcWsl, m.Target))
		}
		if len(mParts) > 0 {
			mountsStr = strings.Join(mParts, ",")
		}
	}

	// 3. Metadata
	// Service Discovery
	hostsContent := ""
	if opts.Name != "" {
		hostsContent += fmt.Sprintf("127.0.0.1 %s\n", opts.Name)
	}

	// List running containers
	// Note: s.List() calls s.wslClient...
	if containers, err := s.List(); err == nil {
		for _, c := range containers {
			if c.Status == "Running" && c.Name != "" {
				hostsContent += fmt.Sprintf("127.0.0.1 %s\n", c.Name)
			}
		}
	}
	if hostsContent != "" {
		s.wslClient.RunDistroCommandWithInput(hostsContent, "sh", "-c", fmt.Sprintf("cat > %s/etc/hosts-extra", rootfsDir))
	}

	meta := Container{
		ID:      containerId,
		Name:    opts.Name,
		Image:   image,
		Command: userCmd,
		Created: time.Now(),
		Status:  "Running",
	}
	metaJSON, _ := json.Marshal(meta)
	s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s/config.json", containerDir))

	// Port Forwarding
	portCmd := ""
	if len(opts.Ports) > 0 {
		portCmd = "command -v socat >/dev/null || apk add --no-cache socat >/dev/null; "
		for _, p := range opts.Ports {
			portCmd += fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr TCP:localhost:%d & ", p.Host, p.Container)
		}
		portCmd = "trap 'JOBS=$(jobs -p); [ -n \"$JOBS\" ] && kill $JOBS' EXIT; " + portCmd
	}

	// 4. Build unshare command
	unshareArgs := []string{
		"unshare", "--mount", "--pid", "--fork", "--uts",
	}

	workdirArg := "none"
	if opts.Workdir != "" {
		workdirArg = opts.Workdir
	}

	if portCmd != "" {
		shellCmd := portCmd + "exec /bin/sh /usr/local/bin/container-shim \"$@\""
		unshareArgs = append(unshareArgs, "sh", "-c", shellCmd, "container-shim", rootfsDir, mountsStr, workdirArg)
	} else {
		unshareArgs = append(unshareArgs, "/bin/sh", "/usr/local/bin/container-shim", rootfsDir, mountsStr, workdirArg)
	}
	unshareArgs = append(unshareArgs, opts.Args...)

	// ENV
	wslEnvList := os.Getenv("WSLENV")
	if opts.Interactive {
		term := os.Getenv("TERM")
		if term == "" {
			term = "xterm-256color"
		}
		os.Setenv("TERM", term)
		if !strings.Contains(wslEnvList, "TERM") {
			wslEnvList = "TERM/u:" + wslEnvList
		}
	}

	for k, v := range opts.Env {
		os.Setenv(k, v)
		if !strings.Contains(wslEnvList, k) {
			wslEnvList = k + "/u:" + wslEnvList
		}
	}
	os.Setenv("WSLENV", wslEnvList)

	// Execution
	if opts.Detach {
		logFile := fmt.Sprintf("%s/console.log", containerDir)
		scriptFile := fmt.Sprintf("%s/run.sh", containerDir)

		var cmdBuilder strings.Builder
		for i, arg := range unshareArgs {
			if i > 0 {
				cmdBuilder.WriteByte(' ')
			}
			escaped := strings.ReplaceAll(arg, "'", "'\\''")
			cmdBuilder.WriteString("'" + escaped + "'")
		}

		scriptContent := fmt.Sprintf("#!/bin/sh\n%s > %s 2>&1\nsed -i 's/\"status\":\"Running\"/\"status\":\"Exited\"/g' %s/config.json",
			cmdBuilder.String(), logFile, containerDir)

		err = s.wslClient.RunDistroCommandWithInput(scriptContent, "sh", "-c", fmt.Sprintf("cat > %s && chmod +x %s", scriptFile, scriptFile))
		if err != nil {
			return fmt.Errorf("failed to create launcher script: %w", err)
		}

		cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "sh", scriptFile)
		err = cmd.Start()
		if err != nil {
			return fmt.Errorf("failed to start detached container: %w", err)
		}
		go func() {
			_ = cmd.Wait()
		}()

		fmt.Printf("Container %s started in background.\n", containerId)
		return nil
	}

	// Normal run
	err = s.wslClient.RunDistroCommand(unshareArgs...)

	meta.Status = "Exited"
	metaJSON, _ = json.Marshal(meta)
	s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s/config.json", containerDir))

	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	return nil
}

func (s *WSLRuntimeService) List() ([]Container, error) {
	findCmd := "find /var/lib/pocketlinx/containers -name config.json"
	cmd := exec.Command("wsl.exe", "-d", DistroName, "--", "sh", "-c", findCmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	var containers []Container
	paths := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		catCmd := exec.Command("wsl.exe", "-d", DistroName, "--", "cat", p)
		catOut, err := catCmd.Output()
		if err != nil {
			continue
		}

		var c Container
		if err := json.Unmarshal(catOut, &c); err == nil {
			containers = append(containers, c)
		}
	}
	return containers, nil
}

func (s *WSLRuntimeService) Stop(id string) error {
	fmt.Printf("Stopping container %s...\n", id)

	stopCmd := fmt.Sprintf("pkill -f 'container-shim.*%s'", id)
	_ = s.wslClient.RunDistroCommand("sh", "-c", stopCmd)

	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	configPath := fmt.Sprintf("%s/config.json", containerDir)

	cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "cat", configPath)
	out, err := cmd.Output()
	if err == nil {
		var meta Container
		if err := json.Unmarshal(out, &meta); err == nil {
			meta.Status = "Exited"
			metaJSON, _ := json.Marshal(meta)
			_ = s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s", configPath))
		}
	}

	fmt.Printf("Container %s stopped.\n", id)
	return nil
}

func (s *WSLRuntimeService) Logs(id string) (string, error) {
	logFile := fmt.Sprintf("/var/lib/pocketlinx/containers/%s/console.log", id)

	cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "cat", logFile)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read logs for container %s (maybe no logs yet): %w", id, err)
	}

	return string(out), nil
}

func (s *WSLRuntimeService) Remove(id string) error {
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	return s.wslClient.RunDistroCommand("rm", "-rf", containerDir)
}

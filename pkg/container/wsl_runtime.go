//go:build windows

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
	network   *BridgeNetworkManager
}

func NewWSLRuntimeService(client *wsl.Client) *WSLRuntimeService {
	// Adapter for CommandRunner
	runner := func(cmd string) (string, error) {
		// Sanitize command for WSL (CRLF -> LF)
		cmd = strings.ReplaceAll(cmd, "\r\n", "\n")
		out, err := exec.Command("wsl.exe", "-d", client.DistroName, "-u", "root", "--", "sh", "-c", cmd).CombinedOutput()
		return string(out), err
	}

	netMgr := NewBridgeNetworkManager(runner)

	// Network initialization is now LAZY (performed in Run/Start) to avoid
	// hanging commands like 'build' or 'images' when the bridge is unstable.

	s := &WSLRuntimeService{
		wslClient: client,
		network:   netMgr,
	}

	// Sync network state (IPs) with existing containers
	if containers, err := s.List(); err == nil {
		for _, c := range containers {
			if c.IP != "" && c.IP != "127.0.0.1" {
				netMgr.MarkIPUsed(c.IP)
				// fmt.Printf("[DEBUG] Reserved IP %s for %s\n", c.IP, c.ID)
			}
		}
	}

	return s
}

func (s *WSLRuntimeService) Run(opts RunOptions) error {
	containerId := fmt.Sprintf("c-%x", time.Now().UnixNano())
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
	// Add explicit ExtraHosts
	for _, h := range opts.ExtraHosts {
		parts := strings.Split(h, ":")
		if len(parts) == 2 {
			hostsContent += fmt.Sprintf("%s %s\n", parts[1], parts[0])
		}
	}
	if hostsContent != "" {
		s.wslClient.RunDistroCommandWithInput(hostsContent, "sh", "-c", fmt.Sprintf("cat > %s/etc/hosts-extra", rootfsDir))
	}

	// 1. Network Setup - Lazy Init
	if err := s.network.SetupBridge(); err != nil {
		fmt.Printf("Warning: Failed to setup network bridge: %v. Networking may not work.\n", err)
	}

	// Allocate IP
	ip, err := s.network.AllocateIP()
	if err != nil {
		return fmt.Errorf("failed to allocate ip: %w", err)
	}

	// Setup Namespace
	if err := s.network.SetupContainerNetwork(containerId, ip); err != nil {
		s.network.ReleaseIP(ip)
		return fmt.Errorf("network setup failed: %w", err)
	}

	meta := Container{
		ID:      containerId,
		Name:    opts.Name,
		Image:   image,
		Command: userCmd,
		Created: time.Now(),
		Status:  "Running",
		Ports:   opts.Ports,
		Config:  opts,
		IP:      ip, // Store IP
	}
	metaJSON, _ := json.Marshal(meta)
	s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s/config.json", containerDir))

	// 4. Port Forwarding - MOVED TO DASHBOARD PROXY
	// s.setupPortForwarding(meta)

	// 4. Build unshare command
	// Wrap with ip netns exec
	unshareArgs := []string{
		"ip", "netns", "exec", containerId,
		"unshare", "--mount", "--pid", "--fork", "--uts",
	}

	workdirArg := "none"
	if opts.Workdir != "" {
		workdirArg = opts.Workdir
	}

	// Simplified shim execution (no internal portCmd)
	unshareArgs = append(unshareArgs, "/bin/sh", "/usr/local/bin/container-shim", rootfsDir, mountsStr, workdirArg)
	unshareArgs = append(unshareArgs, opts.Args...)

	// ENV
	wslEnvList := os.Getenv("WSLENV")
	// ... (env logic same) ...
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
		// We need to update launch script generation to include ip netns exec
		// The simplest way is to update 'generateLaunchScript' or just write the command here for now?
		// generateLaunchScript writes 'run.sh'.
		// We can wrap the call to 'run.sh' with 'ip netns exec ID'.

		if err := s.generateLaunchScript(containerDir, rootfsDir, mountsStr, opts); err != nil {
			return err
		}
		scriptFile := fmt.Sprintf("%s/run.sh", containerDir)

		// Run: wsl -d Distro -- ip netns exec ID sh run.sh
		// Note: 'run.sh' uses 'unshare'.
		// So `ip netns exec ID unshare ...`
		// Wait, `generateLaunchScript` writes `unshare ...`.
		// So we just need to run `sh run.sh` INSIDE netns.

		cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "-u", "root", "--", "ip", "netns", "exec", containerId, "sh", scriptFile)
		err = cmd.Start()
		if err != nil {
			return fmt.Errorf("failed to start detached container: %w", err)
		}
		go func() {
			_ = cmd.Wait()
		}()

		fmt.Printf("Container %s started in background (IP: %s).\n", containerId, ip)
		return nil
	}

	// Normal run
	// RunDistroCommand runs as default user??
	// Network namespace requires root to enter? Or CAP_SYS_ADMIN.
	// Users usually need sudo to enter netns.
	// We might need to prefix "sudo" or run as root.
	// Let's assume root for now (via wslClient setup or sudo).
	// Since we used `exec.Command(..., "-u", "root")` for setup, we should probably do same here.
	// But `RunDistroCommand` might not allow user selection easily.
	// Let's try prepending "sudo"?
	// unshareArgs = append([]string{"sudo"}, unshareArgs...)

	// Actually, `unshare` also needs root/capabilities usually for mount.
	// So existing code probably assumed root or specific setup.

	err = s.wslClient.RunDistroCommand(unshareArgs...)

	meta.Status = "Exited"
	metaJSON, _ = json.Marshal(meta)
	s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s/config.json", containerDir))

	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	return nil
}

func (s *WSLRuntimeService) generateLaunchScript(containerDir, rootfsDir, mountsStr string, opts RunOptions) error {
	// Reconstruct unshare args specifically for the detached script.
	// This duplicates logic unless we share it. But sharing is cleaner.

	// Build unshare command
	unshareArgs := []string{
		"unshare", "--mount", "--pid", "--fork", "--uts",
	}

	workdirArg := "none"
	if opts.Workdir != "" {
		workdirArg = opts.Workdir
	}

	unshareArgs = append(unshareArgs, "/bin/sh", "/usr/local/bin/container-shim", rootfsDir, mountsStr, workdirArg)
	unshareArgs = append(unshareArgs, opts.Args...)

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

	return s.wslClient.RunDistroCommandWithInput(scriptContent, "sh", "-c", fmt.Sprintf("cat > %s && chmod +x %s", scriptFile, scriptFile))
}

func (s *WSLRuntimeService) resolveID(idOrName string) (string, error) {
	// 1. Check if ID directly exists
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", idOrName)
	if err := s.wslClient.RunDistroCommand("test", "-d", containerDir); err == nil {
		return idOrName, nil
	}

	// 2. Scan all configs to find matching Name
	containers, err := s.List()
	if err != nil {
		return "", err
	}

	for _, c := range containers {
		if c.Name == idOrName {
			return c.ID, nil
		}
	}

	return "", fmt.Errorf("container '%s' not found", idOrName)
}

func (s *WSLRuntimeService) Start(idOrName string) error {
	id, err := s.resolveID(idOrName)
	if err != nil {
		return err
	}
	fmt.Printf("Starting container %s...\n", id)

	// Lazy Network Init
	if err := s.network.SetupBridge(); err != nil {
		fmt.Printf("Warning: Failed to setup network bridge: %v. Networking may not work.\n", err)
	}

	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	scriptFile := fmt.Sprintf("%s/run.sh", containerDir)

	// Check if launcher script exists
	if err := s.wslClient.RunDistroCommand("test", "-f", scriptFile); err != nil {
		return fmt.Errorf("container %s cannot be started (no launcher script found)", id)
	}

	// Update status to Running BEFORE starting
	configPath := fmt.Sprintf("%s/config.json", containerDir)
	out, err := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "cat", configPath).Output()
	if err == nil {
		var meta Container
		if err := json.Unmarshal(out, &meta); err == nil {
			meta.Status = "Running"
			metaJSON, _ := json.Marshal(meta)
			_ = s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s", configPath))

		}
	}

	// 3. Execution: Wrap with 'ip netns exec' and run as root
	// Also use setsid and nohup to ensure it detaches from the CLI/Dashboard process.
	// We use the ID (actual directory name) for netns.
	startCmd := fmt.Sprintf("setsid nohup ip netns exec %s sh %s >/dev/null 2>&1 &", id, scriptFile)

	cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "-u", "root", "--", "sh", "-c", startCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

func (s *WSLRuntimeService) List() ([]Container, error) {
	// 圧倒的高速化: find + exec cat を1回の wsl.exe 呼び出しで完結させる
	cmdText := "find /var/lib/pocketlinx/containers -name config.json -exec cat {} +"
	cmd := exec.Command("wsl.exe", "-d", DistroName, "--", "sh", "-c", cmdText)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	var containers []Container
	// 解読: 連結されたJSONをデコード
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var c Container
		if err := dec.Decode(&c); err == nil {
			containers = append(containers, c)
		}
	}
	return containers, nil
}

func (s *WSLRuntimeService) Stop(idOrName string) error {
	id, err := s.resolveID(idOrName)
	if err != nil {
		return err
	}
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	configPath := fmt.Sprintf("%s/config.json", containerDir)
	rootfsDir := fmt.Sprintf("%s/rootfs", containerDir)

	// 1. Kill everything that has the container ID in its command line or process name

	// This includes: launcher script, unshare, shim, and tagged socat processes
	stopCmd := fmt.Sprintf("pkill -9 -f '%s' || true", id)
	_ = s.wslClient.RunDistroCommand("sh", "-c", stopCmd)

	// 2. Kill anything remaining inside the container's rootfs (orphaned internal processes)
	killScript := fmt.Sprintf(`
		for pid_dir in /proc/[0-9]*; do
			[ -d "$pid_dir" ] || continue
			if [ "$(readlink "$pid_dir/root" 2>/dev/null)" = "%s" ]; then
				kill -9 "${pid_dir##*/}" 2>/dev/null || true
			fi
		done
	`, rootfsDir)
	_ = s.wslClient.RunDistroCommand("sh", "-c", killScript)

	// 3. Clean up Port Forwarding Rules (socat proxies)
	stopProxyCmd := fmt.Sprintf("pkill -9 -f 'socat .* %s' || true", id)
	_ = s.wslClient.RunDistroCommand("sh", "-c", stopProxyCmd)

	// Update metadata status

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

func (s *WSLRuntimeService) Logs(idOrName string) (string, error) {
	id, err := s.resolveID(idOrName)
	if err != nil {
		return "", err
	}
	logFile := fmt.Sprintf("/var/lib/pocketlinx/containers/%s/console.log", id)

	cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "cat", logFile)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read logs for container %s (maybe no logs yet): %w", id, err)
	}

	return string(out), nil
}

func (s *WSLRuntimeService) Remove(idOrName string) error {
	id, err := s.resolveID(idOrName)
	if err != nil {
		return err
	}
	// First gather IP to release it
	ip, _ := s.GetIP(id) // Best effort

	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	err = s.wslClient.RunDistroCommand("rm", "-rf", containerDir)

	// Cleanup Network
	if ip != "" && ip != "127.0.0.1" {
		if netErr := s.network.CleanupContainerNetwork(id, ip); netErr != nil {
			fmt.Printf("Warning: failed to cleanup network for %s: %v\n", id, netErr)
		}
	}
	return err
}

func (s *WSLRuntimeService) GetIP(idOrName string) (string, error) {
	id, err := s.resolveID(idOrName)
	if err != nil {
		return "", err
	}
	// Read from config.json
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	configPath := fmt.Sprintf("%s/config.json", containerDir)

	out, err := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "cat", configPath).Output()
	if err != nil {
		// Fallback for old containers or failures
		return "127.0.0.1", fmt.Errorf("failed to read config: %w", err)
	}

	var meta Container
	if err := json.Unmarshal(out, &meta); err != nil {
		return "127.0.0.1", fmt.Errorf("bad config json: %w", err)
	}

	if meta.IP != "" {
		return meta.IP, nil
	}
	return "127.0.0.1", nil
}

func (s *WSLRuntimeService) Update(idOrName string, opts RunOptions) error {
	id, err := s.resolveID(idOrName)
	if err != nil {
		return err
	}
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	configPath := fmt.Sprintf("%s/config.json", containerDir)

	// 1. Read existing config to preserve immutable fields if needed (like Image, ID, Created)
	out, err := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "cat", configPath).Output()
	if err != nil {
		return fmt.Errorf("failed to read container config: %w", err)
	}

	var meta Container
	if err := json.Unmarshal(out, &meta); err != nil {
		return fmt.Errorf("failed to parse container config: %w", err)
	}

	// 2. Apply updates
	// Update command string for display
	meta.Command = strings.Join(opts.Args, " ")
	// Update ports metadata
	meta.Ports = opts.Ports
	// Update Config struct for future edits/clones
	// We should probably merge other fields if opts is incomplete,
	// but for now we assume opts contains the full desired run state (except maybe mounts/env if not passed)
	// The API should probably pass the FULL opts.
	// But let's assume the UI sends the important stuff.
	// Preserving Image/Name from original if empty in opts might be safe, but UI sends them.
	if opts.Image == "" {
		opts.Image = meta.Image
	}
	if opts.Name == "" {
		opts.Name = meta.Name
	}
	meta.Name = opts.Name // Actually apply the name update!
	meta.Config = opts

	// 3. Save updated config
	metaJSON, _ := json.Marshal(meta)
	if err := s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s", configPath)); err != nil {
		return fmt.Errorf("failed to save updated config: %w", err)
	}

	// 4. Regenerate run.sh
	// We need to reconstruct paths
	rootfsDir := path.Join(containerDir, "rootfs")

	// We need to re-calculate Mounts string. Use the logic from Run?
	// The `generateLaunchScript` needs `mountsStr`.
	// We haven't stored `mountsStr` in config.json...
	// We only stored `Mounts` in `meta.Config` (RunOptions).
	// So we can rebuild `mountsStr` from `meta.Config.Mounts`.

	mountsStr := "none"
	if len(opts.Mounts) > 0 {
		var mParts []string
		for _, m := range opts.Mounts {
			var srcWsl string
			isPath := strings.Contains(m.Source, "/") || strings.Contains(m.Source, "\\") || strings.Contains(m.Source, ".")
			if !isPath {
				srcWsl = path.Join(GetWslVolumesDir(), m.Source)
			} else {
				// We can't easily re-resolve Windows paths here without wsl.WindowsToWslPath
				// But we assume the UI passes them back or we use what's in opts.
				// If `opts` comes from `meta.Config`, they might be raw strings.
				// Wait, `RunOptions` in `backend.go` has `Mounts []Mount`.
				// If we saved it to Config, we have it.
				// But we need to convert windows paths again if they are windows paths.
				// Or, we assume they are already WSL paths if they were saved?
				// No, Config stores origin input.
				// Let's rely on the original logic.
				// Ideally, we shouldn't change Mounts in "Edit Command".
				// IF mounts are empty in opts, maybe we should skip regenerating them?
				// But we need the string for the script.

				absSource, _ := filepath.Abs(m.Source)
				// Re-conversion might be needed.
				if converted, err := wsl.WindowsToWslPath(absSource); err == nil {
					srcWsl = converted
				} else {
					srcWsl = m.Source // Fallback
				}
			}
			mParts = append(mParts, fmt.Sprintf("%s:%s", srcWsl, m.Target))
		}
		if len(mParts) > 0 {
			mountsStr = strings.Join(mParts, ",")
		}
	}

	if err := s.generateLaunchScript(containerDir, rootfsDir, mountsStr, opts); err != nil {
		return fmt.Errorf("failed to regenerate launch script: %w", err)
	}

	fmt.Printf("Container %s configuration updated.\n", id)
	return nil
}

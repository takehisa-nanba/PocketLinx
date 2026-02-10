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
	hostIP    string // Added to store detected host IP
}

func NewWSLRuntimeService(client *wsl.Client) *WSLRuntimeService {
	// Adapter for CommandRunner
	runner := func(cmd string) (string, error) {
		if os.Getenv("PLX_VERBOSE") != "" {
			fmt.Printf("[DEBUG] WSL Distro Command: %s\n", cmd)
		}
		// Sanitize command for WSL (CRLF -> LF)
		cmd = strings.ReplaceAll(cmd, "\r\n", "\n")
		out, err := exec.Command("wsl.exe", "-d", client.DistroName, "-u", "root", "--", "sh", "-c", cmd).CombinedOutput()
		return string(out), err
	}

	// Default to plx0/10.10.0.0/24 (Compatible with existing installations)
	netMgr := NewBridgeNetworkManager(runner, "plx0", "10.10.0.0/24")

	s := &WSLRuntimeService{
		wslClient: client,
		network:   netMgr,
		hostIP:    "", // Lazy detection
	}

	// Recover IP state from existing containers (v0.7.18)
	s.recoverNetworkState()

	return s
}

func (s *WSLRuntimeService) detectHostIP() {
	if s.hostIP != "" {
		return
	}
	if os.Getenv("PLX_VERBOSE") != "" {
		fmt.Printf("[DEBUG] Detecting host IP via gateway...\n")
	}
	// Detect host IP (WSL Gateway)
	// Use more robust command to get ONLY the gateway IP
	out, err := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "-u", "root", "--", "sh", "-c", "ip route show | grep default | grep -oE '[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+' | head -n1").CombinedOutput()
	if err == nil {
		s.hostIP = strings.TrimSpace(string(out))
	}
	if s.hostIP == "" {
		s.hostIP = "127.0.0.1"
	}
	if os.Getenv("PLX_VERBOSE") != "" {
		fmt.Printf("[DEBUG] Detected Host IP: %s\n", s.hostIP)
	}
}

func (s *WSLRuntimeService) recoverNetworkState() {
	if os.Getenv("PLX_VERBOSE") != "" {
		fmt.Println("[DEBUG] Recovering network state from existing containers...")
	}
	containers, err := s.List()
	if err != nil {
		fmt.Printf("Warning: Failed to recover network state: %v. IP conflicts may occur.\n", err)
		return
	}
	for _, c := range containers {
		// Mark IP as used if it's assigned to an existing container (v0.7.18)
		if c.IP != "" {
			s.network.MarkIPUsed(c.IP)
		}
	}
}

func (s *WSLRuntimeService) Run(opts RunOptions) error {
	containerId := opts.Name
	if containerId == "" {
		containerId = fmt.Sprintf("c-%x", time.Now().UnixNano())
	}
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
	wslImgMetaPath := path.Join(GetWslImagesDir(), image+".json")

	// Check existence in WSL
	if err := s.wslClient.RunDistroCommand("test", "-f", wslImgPath); err != nil {
		return fmt.Errorf("image '%s' not found. Please run 'plx pull %s' first", image, image)
	}

	// Load Image Metadata for defaults (v0.7.3)
	if data, err := s.wslClient.RunDistroCommandOutput("cat", wslImgMetaPath); err == nil {
		var imgMeta ImageMetadata
		if err := json.Unmarshal([]byte(data), &imgMeta); err == nil {
			if opts.User == "" {
				opts.User = imgMeta.User
			}
			if opts.Workdir == "" {
				opts.Workdir = imgMeta.Workdir
			}
			if opts.Env == nil {
				opts.Env = make(map[string]string)
			}
			for k, v := range imgMeta.Env {
				if _, exists := opts.Env[k]; !exists {
					opts.Env[k] = v
				}
			}
			if len(opts.Args) == 0 && len(imgMeta.Command) > 0 {
				opts.Args = imgMeta.Command
				fmt.Printf("Using default command: %v\n", opts.Args)
			}
		}
	}

	wslRootfsPath := wslImgPath
	var err error

	// 1. Provisioning
	s.wslClient.RunDistroCommand("mkdir", "-p", containerDir, rootfsDir)

	// 1. Provisioning Rootfs
	if err := s.provisionRootfs(wslRootfsPath, containerDir, rootfsDir); err != nil {
		return err
	}

	// Update Shim
	if err := s.wslClient.RunDistroCommandWithInput(shim.Content, "sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim"); err != nil {
		fmt.Printf("Warning: Failed to update shim: %v\n", err)
	}

	// 2. Process Mounts
	mountsStr := "none"
	var volumeMkdirCmds []string
	if len(opts.Mounts) > 0 {
		var mParts []string
		for i, m := range opts.Mounts {
			var srcWsl string
			isPath := strings.Contains(m.Source, "/") || strings.Contains(m.Source, "\\") || strings.Contains(m.Source, ".")

			if !isPath {
				// Named Volume
				volName := m.Source
				srcWsl = path.Join(GetWslVolumesDir(), volName)
				volumeMkdirCmds = append(volumeMkdirCmds, fmt.Sprintf("mkdir -p %s", srcWsl))
			} else {
				// Bind Mount
				absSource, _ := filepath.Abs(m.Source)
				// Update opts so metadata shows absolute path (v0.7.17)
				opts.Mounts[i].Source = absSource

				// Auto-create host directory if it doesn't exist (v0.7.14)
				if _, err := os.Stat(absSource); os.IsNotExist(err) {
					_ = os.MkdirAll(absSource, 0755)
				}
				var err error
				srcWsl, err = wsl.WindowsToWslPath(absSource)
				if err != nil {
					fmt.Printf("Warning: Failed to convert mount path %s: %v\n", absSource, err)
					continue
				}
			}
			mParts = append(mParts, fmt.Sprintf("%s:%s", srcWsl, m.Target))
		}
		if len(mParts) > 0 {
			mountsStr = strings.Join(mParts, ",")
		}
	}

	// 2. Setup Network
	ip, netScript, err := s.setupNetwork(containerId)
	if err != nil {
		return err
	}

	// 3. metadata for config.json (now we have IP)
	meta := Container{
		ID:      containerId,
		Name:    opts.Name,
		Image:   image,
		Command: userCmd,
		Created: time.Now(),
		Status:  "Running",
		Ports:   opts.Ports,
		Config:  opts,
		IP:      ip,
	}
	metaJSON, _ := json.Marshal(meta)

	// 4. Final Configuration (Hosts, Metadata, Shim, Volumes)
	hostsContent := s.generateHostsContent(opts)
	if err := s.configureContainer(containerDir, rootfsDir, string(metaJSON), hostsContent, volumeMkdirCmds, netScript); err != nil {
		s.network.ReleaseIP(ip)
		return err
	}

	// 5. Execution
	return s.executeContainer(containerId, containerDir, rootfsDir, opts, mountsStr, ip, meta)
}

func (s *WSLRuntimeService) generateHostsContent(opts RunOptions) string {
	content := ""
	if opts.Name != "" {
		content += fmt.Sprintf("127.0.0.1 %s\n", opts.Name)
	}
	s.detectHostIP()
	if s.hostIP != "" {
		content += fmt.Sprintf("%s host.plx.internal\n", s.hostIP)
	}
	for _, h := range opts.ExtraHosts {
		parts := strings.Split(h, ":")
		if len(parts) == 2 {
			content += fmt.Sprintf("%s %s\n", parts[1], parts[0])
		}
	}
	return content
}

func (s *WSLRuntimeService) provisionRootfs(wslRootfsPath, containerDir, rootfsDir string) error {
	s.wslClient.RunDistroCommand("mkdir", "-p", containerDir, rootfsDir)
	fmt.Printf("Provisioning container filesystems (extracting rootfs)...\n")
	startProv := time.Now()

	provCmd := s.wslClient.PrepareDistroCommand("tar", "-xzf", wslRootfsPath, "-C", rootfsDir)
	if err := provCmd.Start(); err != nil {
		return fmt.Errorf("failed to start provisioning: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- provCmd.Wait() }()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("provisioning failed: %w", err)
			}
			fmt.Printf("\x1b[2K\rProvisioning container filesystems... done. (%s)\n", time.Since(startProv).Round(time.Second))
			return nil
		case <-ticker.C:
			fmt.Printf("\x1b[2K\rProvisioning... (%ds elapsed)", int(time.Since(startProv).Seconds()))
		}
	}
}

func (s *WSLRuntimeService) setupNetwork(containerId string) (string, string, error) {
	ip, err := s.network.AllocateIP()
	if err != nil {
		return "", "", fmt.Errorf("failed to allocate ip: %w", err)
	}
	fmt.Printf("Allocating network and IP (%s)... ", ip)
	if err := s.network.SetupBridge(); err != nil {
		return "", "", fmt.Errorf("failed to setup network bridge: %w", err)
	}
	fmt.Println("done.")

	netScript, _, _ := s.network.GetSetupScript(containerId, ip)
	return ip, netScript, nil
}

func (s *WSLRuntimeService) configureContainer(containerDir, rootfsDir, metaJSON, hostsContent string, volumeMkdirCmds []string, netScript string) error {
	configScript := fmt.Sprintf(`
set -e
# A. Essential Metadata & Shim
cat <<'EOF' > %s/config.json
%s
EOF
cat <<'EOF' > /usr/local/bin/container-shim
%s
EOF
chmod +x /usr/local/bin/container-shim

# B. Hosts & Network
mkdir -p %s/etc
cat <<'EOF' > %s/etc/hosts-extra
%s
EOF

# C. Volumes
%s

# D. Network Bridge
%s
`, containerDir, metaJSON, shim.Content, rootfsDir, rootfsDir, hostsContent, strings.Join(volumeMkdirCmds, "\n"), netScript)

	return s.wslClient.RunDistroCommandWithInput(configScript, "sh", "-e")
}

func (s *WSLRuntimeService) executeContainer(containerId, containerDir, rootfsDir string, opts RunOptions, mountsStr, ip string, meta Container) error {
	userArg := "none"
	if opts.User != "" {
		userArg = opts.User
	}
	workdirArg := "none"
	if opts.Workdir != "" {
		workdirArg = opts.Workdir
	}

	unshareArgs := []string{
		"ip", "netns", "exec", containerId,
		"unshare", "--mount", "--pid", "--fork", "--uts",
	}
	pidFile := path.Join(containerDir, "shim.pid")
	unshareArgs = append(unshareArgs, "/bin/sh", "/usr/local/bin/container-shim", rootfsDir, mountsStr, workdirArg, userArg, pidFile)
	unshareArgs = append(unshareArgs, opts.Args...)

	// ENV
	wslEnvList := os.Getenv("WSLENV")
	if opts.Interactive {
		if os.Getenv("TERM") == "" {
			os.Setenv("TERM", "xterm-256color")
		}
		if !strings.Contains(wslEnvList, "TERM") {
			wslEnvList = "TERM/u:" + wslEnvList
		}
	}
	for k, v := range opts.Env {
		envKey := k
		if k == "PATH" {
			envKey = "PLX_CONTAINER_PATH"
		}
		os.Setenv(envKey, v)
		if !strings.Contains(wslEnvList, envKey) {
			wslEnvList = envKey + "/u:" + wslEnvList
		}
	}
	os.Setenv("WSLENV", wslEnvList)

	if opts.Detach {
		if err := s.generateLaunchScript(containerDir, rootfsDir, mountsStr, opts); err != nil {
			return err
		}
		scriptFile := path.Join(containerDir, "run.sh")
		cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "-u", "root", "--", "ip", "netns", "exec", containerId, "sh", scriptFile)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start detached container: %w", err)
		}
		go func() { _ = cmd.Wait() }()
		fmt.Printf("Container %s started in background (IP: %s).\n", containerId, ip)
		return nil
	}

	err := s.wslClient.RunDistroCommand(unshareArgs...)

	// Update status
	meta.Status = "Exited"
	metaJSON, _ := json.Marshal(meta)
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

	pidFile := fmt.Sprintf("%s/shim.pid", containerDir)

	userArg := "none"
	if opts.User != "" {
		userArg = opts.User
	}

	unshareArgs = append(unshareArgs, "/bin/sh", "/usr/local/bin/container-shim", rootfsDir, mountsStr, workdirArg, userArg, pidFile)
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

func (s *WSLRuntimeService) Exec(idOrName string, cmdArgs []string, interactive bool) error {
	id, err := s.resolveID(idOrName)
	if err != nil {
		return err
	}
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	rootfsDir := fmt.Sprintf("%s/rootfs", containerDir)

	// Execute inside the container's namespaces using nsenter
	// 1. Find the parent PID (unshare process)
	// Find parent PID (the shim)
	// Tighten regex to avoid partial matches (v0.7.17)
	parentPidStr, err := s.wslClient.RunDistroCommandOutput("pgrep", "-f", fmt.Sprintf("container-shim %s ", rootfsDir))
	if err != nil || strings.TrimSpace(parentPidStr) == "" {
		// Fallback...
		parentPidStr, err = s.wslClient.RunDistroCommandOutput("sh", "-c", fmt.Sprintf("ps -o pid,args | grep 'container-shim.*%s/rootfs' | grep -v grep | head -n 1 | awk '{print $1}'", id))
		if err != nil || strings.TrimSpace(parentPidStr) == "" {
			return fmt.Errorf("failed to find container process (is it running?): %w", err)
		}
	}
	// Take the first line (oldest process usually)
	parentPids := strings.Split(strings.TrimSpace(parentPidStr), "\n")
	parentPid := strings.TrimSpace(parentPids[0])

	// 2. Find the child PID (the actual containerized process)
	// pgrep -P PARENT_PID
	childPidStr, err := s.wslClient.RunDistroCommandOutput("pgrep", "-P", parentPid)
	if err != nil || strings.TrimSpace(childPidStr) == "" {
		return fmt.Errorf("failed to find container child process for parent %s: %w", parentPid, err)
	}
	// Take the first child
	childPids := strings.Split(strings.TrimSpace(childPidStr), "\n")
	pid := strings.TrimSpace(childPids[0])

	// Inject common PATHs and host address for ADB
	pathEnv := "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/flutter/bin:/opt/android-sdk/platform-tools"
	adbEnv := "ANDROID_ADB_SERVER_ADDRESS=host.plx.internal"

	// Simplified execution:
	// We don't use 'exec' here to support commands with semicolons correctly
	userCmd := strings.Join(cmdArgs, " ")
	shCmd := fmt.Sprintf("export %s %s; %s", pathEnv, adbEnv, userCmd)

	// nsenter -t PID -m -n -u -i -p joins namespaces (mount, net, uts, ipc, pid)
	// We DON'T use -r (root) because the proc's root may show as "(deleted)" due to mount namespace changes
	// Instead, we chroot explicitly to the container's rootfs directory
	rootfsPath := fmt.Sprintf("/var/lib/pocketlinx/containers/%s/rootfs", id)
	args := []string{"-d", s.wslClient.DistroName, "-u", "root", "--", "nsenter", "-t", pid, "-m", "-n", "-u", "-i", "-p", "--", "chroot", rootfsPath, "/bin/sh", "-c", shCmd}

	if os.Getenv("PLX_VERBOSE") != "" {
		fmt.Printf("[DEBUG] Executing in container %s: %v\n", id, cmdArgs)
	}

	cmd := exec.Command("wsl.exe", args...)
	if interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("exec failed: %s", string(out))
	}
	fmt.Print(string(out))
	return nil
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
	var ip string
	if err == nil {
		var meta Container
		if err := json.Unmarshal(out, &meta); err == nil {
			meta.Status = "Running"
			ip = meta.IP // Retrieve IP from config
			metaJSON, _ := json.Marshal(meta)
			_ = s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s", configPath))
		}
	}

	// 2.5 Ensure Network is configured (for persistent/restart)
	if ip != "" {
		netScript, _, err := s.network.GetSetupScript(id, ip)
		if err == nil {
			// Execute network setup script AS ROOT because it creates interfaces
			cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "-u", "root", "--", "sh", "-e")
			// Sanitize input
			netScript = strings.ReplaceAll(netScript, "\r\n", "\n")
			cmd.Stdin = strings.NewReader(netScript)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("Warning: Failed to setup network for restart: %v\n", err)
			}
		}
	}

	// 2.6 Inject host.plx.internal for ADB and host connectivity
	rootfsDir := fmt.Sprintf("%s/rootfs", containerDir)
	gatewayIP, err := s.wslClient.RunDistroCommandOutput("sh", "-c", "ip route show | grep default | cut -d' ' -f3")
	if err == nil && strings.TrimSpace(gatewayIP) != "" {
		hostEntry := fmt.Sprintf("%s host.plx.internal\n", strings.TrimSpace(gatewayIP))
		hostsExtraPath := fmt.Sprintf("%s/etc/hosts-extra", rootfsDir)
		_ = s.wslClient.RunDistroCommandWithInput(hostEntry, "sh", "-c", fmt.Sprintf("cat > %s", hostsExtraPath))
	}

	// 3. Execution: Wrap with 'ip netns exec' and run as root
	// Use daemonize pattern: nohup runs in subshell which is immediately disowned
	// The double-fork pattern ensures the process survives WSL session termination
	startCmd := fmt.Sprintf("nohup ip netns exec %s sh %s >%s/console.log 2>&1 </dev/null &", id, scriptFile, containerDir)

	cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "-u", "root", "--", "sh", "-c", startCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

func (s *WSLRuntimeService) List() ([]Container, error) {
	// 圧倒的高速化: find + exec cat を1回の wsl.exe 呼び出しで完結させる
	cmdText := "find /var/lib/pocketlinx/containers -name config.json -exec cat {} +"
	cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "--", "sh", "-c", cmdText)
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

	// 1. Read PID from file if exists (v0.8.0)
	pidFile := path.Join(containerDir, "shim.pid")
	pidBytes, err := s.wslClient.RunDistroCommandOutput("cat", pidFile)
	if err == nil && strings.TrimSpace(pidBytes) != "" {
		pid := strings.TrimSpace(pidBytes)
		// Verify if the PID is still alive and belongs to the shim (v0.8.1)
		// kill -0 just checks existence.
		if err := s.wslClient.RunDistroCommand("kill", "-0", pid); err == nil {
			// Check process context to be absolutely sure (v0.8.1)
			cmdline, _ := s.wslClient.RunDistroCommandOutput("cat", fmt.Sprintf("/proc/%s/cmdline", pid))
			if strings.Contains(cmdline, "container-shim") && strings.Contains(cmdline, id) {
				_ = s.wslClient.RunDistroCommand("kill", "-9", pid)
			}
		}
	}

	// 2. Kill everything that has the container ID in its command line or process name
	// Use more specific pattern to avoid 'c1' matches 'c11' (v0.8.0)
	// We look for 'container-shim.*ID/rootfs' or 'netns exec ID'
	stopCmd := fmt.Sprintf("pkill -9 -f 'container-shim.*%s/rootfs' || true", id)
	_ = s.wslClient.RunDistroCommand("sh", "-c", stopCmd)
	_ = s.wslClient.RunDistroCommand("sh", "-c", fmt.Sprintf("pkill -9 -f 'ip netns exec %s' || true", id))

	// 3. Kill anything remaining inside the container's rootfs (orphaned internal processes)

	// 4. Clean up Mounts (v0.8.0)
	// /proc/mounts を解析し、rootfsDir 以下のすべてのマウントポイントを特定して、
	// 依存関係を考慮した逆順（深い順）で強制アンマウント（-l）する。
	// grep パターンと while 内の変数をクォートしてスペースに対応 (v0.8.1)
	unmountScript := fmt.Sprintf(`
		grep " %s" /proc/mounts | cut -d' ' -f2- | while read -r mnt; do
			# Get the actual mount point (handle potential trailing spaces or escaped chars)
			# Strip anything after the mount point if cut -f2- was too greedy
			mnt_clean=$(echo "$mnt" | awk '{print $1}')
			[ -n "$mnt_clean" ] && umount -l "$mnt_clean" 2>/dev/null || true
		done
	`, rootfsDir)
	_ = s.wslClient.RunDistroCommand("sh", "-c", unmountScript)

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

	// We rebuild mountsStr from opts.Mounts.
	// IMPORTANT: If opts.Mounts is empty, we might accidentally clear mounts.
	// But plx update --args doesn't necessarily pass mounts.
	// Merge with existing mounts if empty (v0.8.1)
	if len(opts.Mounts) == 0 {
		opts.Mounts = meta.Config.Mounts
	}

	mountsStr := "none"
	if len(opts.Mounts) > 0 {
		var mParts []string
		for _, m := range opts.Mounts {
			var srcWsl string
			isPath := strings.Contains(m.Source, "/") || strings.Contains(m.Source, "\\") || strings.Contains(m.Source, ".")
			if !isPath {
				srcWsl = path.Join(GetWslVolumesDir(), m.Source)
			} else {
				// Use the stored source path. If it's already absolute (saved by Run),
				// do NOT apply filepath.Abs relative to the current plx update location.
				// This avoids the "CWD Trap" (v0.8.1)
				src := m.Source
				if !filepath.IsAbs(src) && !strings.HasPrefix(src, "\\\\") {
					abs, _ := filepath.Abs(src)
					src = abs
				}

				if converted, err := wsl.WindowsToWslPath(src); err == nil {
					srcWsl = converted
				} else {
					srcWsl = src // Fallback
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

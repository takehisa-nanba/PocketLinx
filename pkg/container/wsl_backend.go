package container

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"PocketLinx/pkg/shim"
	"PocketLinx/pkg/wsl"
)

// WSLBackend はWSL2を利用したコンテナ実行バックエンドです。
type WSLBackend struct {
	wslClient *wsl.Client
}

func NewWSLBackend() *WSLBackend {
	return &WSLBackend{
		wslClient: wsl.NewClient(DistroName),
	}
}

func (b *WSLBackend) Install() error {
	return InstallBinary()
}

func (b *WSLBackend) Setup() error {
	fmt.Println("Setting up PocketLinx environment (WSL2)...")

	// 1. Host side directories
	_ = GetImagesDir()
	_ = GetDistroDir()

	// 2. Distro side initialization
	fmt.Println("Fixing filesystem and network in pocketlinx distro... (PATCHED)")
	initCmds := `
		# A. Fix WSL Configuration to prevent automatic overwrites
		cat <<EOF > /etc/wsl.conf
[network]
generateResolvConf = false
[interop]
enabled = true
appendWindowsPath = true
EOF

		# B. Fix DNS first
		echo "nameserver 8.8.8.8" > /etc/resolv.conf
		echo "nameserver 1.1.1.1" >> /etc/resolv.conf

		# C. Update and Install core tools
		apk update
		apk add --no-cache tzdata util-linux socat

		# D. Set Timezone (Copy instead of link for early boot stability)
		if [ -f /usr/share/zoneinfo/Asia/Tokyo ]; then
			cp /usr/share/zoneinfo/Asia/Tokyo /etc/localtime
			echo "Asia/Tokyo" > /etc/timezone
		fi

		# E. Satisfy WSL init process (Fix ldconfig failed error)
		# 最後に実行することで、apkによる上書きを確実に防ぐ
		mkdir -p /etc/ld.so.conf.d
		rm -f /sbin/ldconfig /usr/sbin/ldconfig
		printf "#!/bin/sh\nexit 0\n" | tr -d '\r' > /sbin/ldconfig
		chmod +x /sbin/ldconfig
		cp /sbin/ldconfig /usr/sbin/ldconfig

		# 設定を確実に反映
		sync
	`
	if err := b.wslClient.RunDistroCommand("sh", "-c", initCmds); err != nil {
		return fmt.Errorf("failed to initialize pocketlinx distro: %w", err)
	}

	fmt.Println("Environment is now healthy.")
	return nil
}

func (b *WSLBackend) Pull(image string) error {
	url, ok := SupportedImages[image]
	if !ok {
		return fmt.Errorf("image '%s' is not supported", image)
	}

	wslImagesDir := GetWslImagesDir()

	// Special handling for System Distro bootstrap (Alpine)
	if image == "alpine" {
		targetFile := filepath.Join(GetImagesDir(), image+".tar.gz")
		if _, err := os.Stat(targetFile); err != nil {
			fmt.Printf("Downloading bootstrap image '%s'...\n", image)
			cmd := exec.Command("powershell.exe", "-Command", fmt.Sprintf("Invoke-WebRequest -Uri %s -OutFile %s", url, targetFile))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("error downloading bootstrap image: %w", err)
			}
		}

		installDir := GetDistroDir()
		absInstallDir, _ := filepath.Abs(installDir)
		absRootfsFile, _ := filepath.Abs(targetFile)

		fmt.Printf("Importing system distro '%s'...\n", DistroName)
		b.wslClient.Run("--unregister", DistroName)
		os.RemoveAll(absInstallDir)
		os.MkdirAll(absInstallDir, 0755)

		if err := b.wslClient.Run("--import", DistroName, absInstallDir, absRootfsFile, "--version", "2"); err != nil {
			return fmt.Errorf("error importing system distro: %w", err)
		}

		fmt.Println("Installing container-shim...")
		if err := b.wslClient.RunDistroCommandWithInput(shim.Content, "sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim"); err != nil {
			return err
		}

		// Distro exists now. Ensure directory exists inside WSL
		if err := b.wslClient.RunDistroCommand("mkdir", "-p", wslImagesDir); err != nil {
			return fmt.Errorf("failed to create images dir in WSL: %w", err)
		}

		// Cache this image into WSL storage for Run/Build to use
		fmt.Println("Caching bootstrap image to WSL storage...")
		wslWinPath, _ := wsl.WindowsToWslPath(targetFile)
		targetWslFile := path.Join(wslImagesDir, image+".tar.gz")
		b.wslClient.RunDistroCommand("cp", wslWinPath, targetWslFile)
		return nil
	}

	// Normal flow for other images (Distro assumed to exist)
	// Ensure directory exists
	if err := b.wslClient.RunDistroCommand("mkdir", "-p", wslImagesDir); err != nil {
		return fmt.Errorf("failed to create images dir in WSL (is PocketLinx setup?): %w", err)
	}
	targetWslFile := path.Join(wslImagesDir, image+".tar.gz")

	// Check if exists in WSL
	if err := b.wslClient.RunDistroCommand("test", "-f", targetWslFile); err == nil {
		fmt.Printf("Image '%s' already exists.\n", image)
		return nil
	}

	// Native Download for other images
	fmt.Printf("Pulling image '%s' inside WSL...\n", image)
	downloadCmd := fmt.Sprintf("wget -O %s %s || curl -L -o %s %s", targetWslFile, url, targetWslFile, url)
	if err := b.wslClient.RunDistroCommand("sh", "-c", downloadCmd); err != nil {
		return fmt.Errorf("error downloading image in WSL: %w", err)
	}

	return nil
}

func (b *WSLBackend) Images() ([]string, error) {
	// List files in WSL images directory
	cmd := fmt.Sprintf("ls %s/*.tar.gz", GetWslImagesDir())
	out, err := b.wslClient.RunDistroCommandOutput("sh", "-c", cmd)
	if err != nil {
		// If explicit ls fails, it might mean empty dir or no files
		return []string{}, nil
	}

	var images []string
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		// /var/lib/pocketlinx/images/alpine.tar.gz -> alpine
		base := path.Base(l)
		if strings.HasSuffix(base, ".tar.gz") {
			images = append(images, strings.TrimSuffix(base, ".tar.gz"))
		}
	}
	return images, nil
}

func (b *WSLBackend) Run(opts RunOptions) error {
	containerId := fmt.Sprintf("c-%d", os.Getpid())
	fmt.Printf("Running %v in container %s (WSL2)...\n", opts.Args, containerId)

	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", containerId)
	rootfsDir := path.Join(containerDir, "rootfs")

	// Metadata command string (for display and config.json)
	userCmd := strings.Join(opts.Args, " ")

	image := opts.Image
	if image == "" {
		image = "alpine"
	}

	wslImgPath := path.Join(GetWslImagesDir(), image+".tar.gz")

	// Check existence in WSL
	if err := b.wslClient.RunDistroCommand("test", "-f", wslImgPath); err != nil {
		return fmt.Errorf("image '%s' not found. Please run 'plx pull %s' first", image, image)
	}

	wslRootfsPath := wslImgPath
	var err error // Declared for use in subsequent blocks

	// 1. Provisioning
	// 明示的にコンテナディレクトリ（親）を作成してからrootfsを作成する
	setupCmd := fmt.Sprintf("mkdir -p %s && mkdir -p %s && tar -xf %s -C %s", containerDir, rootfsDir, wslRootfsPath, rootfsDir)
	if err := b.wslClient.RunDistroCommand("sh", "-c", setupCmd); err != nil {
		return fmt.Errorf("provisioning failed (path: %s): %w", containerDir, err)
	}

	// Update Shim to ensure it matches the current binary version
	if err := b.wslClient.RunDistroCommandWithInput(shim.Content, "sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim"); err != nil {
		fmt.Printf("Warning: Failed to update shim: %v\n", err)
	}

	// 2. Process Mounts
	mountsStr := "none"
	if len(opts.Mounts) > 0 {
		var mParts []string
		for _, m := range opts.Mounts {
			var srcWsl string

			// Detect if Named Volume
			// Heuristic: If it has "C:\" or "/" or ".", it's a path. Else assume named volume (simple alphanumeric).
			isPath := strings.Contains(m.Source, "/") || strings.Contains(m.Source, "\\") || strings.Contains(m.Source, ".")

			if !isPath {
				// Named Volume
				volName := m.Source
				srcWsl = path.Join(GetWslVolumesDir(), volName)
				// Ensure volume exists (Auto-create on Run)
				if err := b.wslClient.RunDistroCommand("mkdir", "-p", srcWsl); err != nil {
					fmt.Printf("Warning: Failed to create/ensure volume %s: %v\n", volName, err)
					continue
				}
			} else {
				// Bind Mount (Windows Path)
				// We need to resolve absolute path first because input might be relative "."
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
	// Service Discovery: Scan other containers to build /etc/hosts aliases
	hostsContent := ""
	// Add self
	if opts.Name != "" {
		containerId = opts.Name // Use Name as ID if provided? No, keep ID unique, just use Name as alias.
		// Actually, if Name is provided, we should probably ensure uniqueness?
		// For simplicity v0.3.0, we just trust user or overwrite.
		// Let's keep ID as c-PID but add Name to metadata.
		hostsContent += fmt.Sprintf("127.0.0.1 %s\n", opts.Name)
	}

	// List running containers to add their names
	if containers, err := b.List(); err == nil {
		for _, c := range containers {
			if c.Status == "Running" && c.Name != "" {
				hostsContent += fmt.Sprintf("127.0.0.1 %s\n", c.Name)
			}
		}
	}
	// Write hosts-extra
	if hostsContent != "" {
		b.wslClient.RunDistroCommandWithInput(hostsContent, "sh", "-c", fmt.Sprintf("cat > %s/etc/hosts-extra", rootfsDir))
	}

	meta := Container{
		ID:      containerId,
		Name:    opts.Name,
		Image:   image, // Save Image
		Command: userCmd,
		Created: time.Now(),
		Status:  "Running",
	}
	metaJSON, _ := json.Marshal(meta)
	b.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s/config.json", containerDir))

	// ポート転送コマンドの組み立て
	portCmd := ""
	if len(opts.Ports) > 0 {
		// socat があるか確認し、なければインストール
		portCmd = "command -v socat >/dev/null || apk add --no-cache socat >/dev/null; "
		for _, p := range opts.Ports {
			// socat を起動し、その PID を控えておく
			portCmd += fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr TCP:localhost:%d & ", p.Host, p.Container)
		}
		// 終了時にバックグラウンドプロセスが存在する場合のみ終了させる
		portCmd = "trap 'JOBS=$(jobs -p); [ -n \"$JOBS\" ] && kill $JOBS' EXIT; " + portCmd
	}

	// 4. Build unshare command slice
	unshareArgs := []string{
		"unshare", "--mount", "--pid", "--fork", "--uts",
	}

	workdirArg := "none"
	if opts.Workdir != "" {
		workdirArg = opts.Workdir
	}

	if portCmd != "" {
		// Use sh -c to setup port forwarding, then exec the shim
		// "$@" receives the trailing arguments (rootfs, mounts, workdir, and container command)
		shellCmd := portCmd + "exec /bin/sh /usr/local/bin/container-shim \"$@\""
		unshareArgs = append(unshareArgs, "sh", "-c", shellCmd, "container-shim", rootfsDir, mountsStr, workdirArg)
	} else {
		// No ports, call shim directly
		unshareArgs = append(unshareArgs, "/bin/sh", "/usr/local/bin/container-shim", rootfsDir, mountsStr, workdirArg)
	}
	unshareArgs = append(unshareArgs, opts.Args...)

	// インタラクティブモードや環境変数を WSL 側に渡す設定
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

	// ユーザー指定の環境変数をセット
	for k, v := range opts.Env {
		os.Setenv(k, v)
		if !strings.Contains(wslEnvList, k) {
			wslEnvList = k + "/u:" + wslEnvList
		}
	}
	os.Setenv("WSLENV", wslEnvList)

	// 9. 実行
	if opts.Detach {
		// デタッチモード: 引用符の問題を避けるため、一旦スクリプトに書き出す
		logFile := fmt.Sprintf("%s/console.log", containerDir)
		scriptFile := fmt.Sprintf("%s/run.sh", containerDir)

		// コマンドを安全にエスケープしてスクリプトを作成
		// シングルクォートで囲み、内部のシングルクォートを '\'' に置換する
		var cmdBuilder strings.Builder
		for i, arg := range unshareArgs {
			if i > 0 {
				cmdBuilder.WriteByte(' ')
			}
			escaped := strings.ReplaceAll(arg, "'", "'\\''")
			cmdBuilder.WriteString("'" + escaped + "'")
		}

		// コンテナ実行後にステータスを Exited に更新するスクリプト
		// exec は使わず、終了を待ってから sed で Running を Exited に置換する
		scriptContent := fmt.Sprintf("#!/bin/sh\n%s > %s 2>&1\nsed -i 's/\"status\":\"Running\"/\"status\":\"Exited\"/g' %s/config.json",
			cmdBuilder.String(), logFile, containerDir)

		err = b.wslClient.RunDistroCommandWithInput(scriptContent, "sh", "-c", fmt.Sprintf("cat > %s && chmod +x %s", scriptFile, scriptFile))
		if err != nil {
			return fmt.Errorf("failed to create launcher script: %w", err)
		}

		// バックグラウンドで実行開始 (Host側で非同期に起動)
		// WSL内で & でバックグラウンド化するのではなく、Host側でプロセスとして切り離す
		cmd := exec.Command("wsl.exe", "-d", b.wslClient.DistroName, "--", "sh", scriptFile)
		err = cmd.Start()
		if err != nil {
			return fmt.Errorf("failed to start detached container: %w", err)
		}

		// 完全に切り離すために、Go側のハンドルを閉じる（任意だが安全のため）
		go func() {
			_ = cmd.Wait() // 背景で待機してリソース回収
		}()

		fmt.Printf("Container %s started in background.\n", containerId)
		return nil
	}

	// 通常モード: 実行を待機
	err = b.wslClient.RunDistroCommand(unshareArgs...)

	// 終了後のステータス更新（デタッチ時はここに来ない）
	meta.Status = "Exited"
	metaJSON, _ = json.Marshal(meta)
	b.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s/config.json", containerDir))

	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	return nil
}

func (b *WSLBackend) List() ([]Container, error) {
	// config.json のパス一覧を取得
	findCmd := "find /var/lib/pocketlinx/containers -name config.json"
	cmd := exec.Command("wsl.exe", "-d", DistroName, "--", "sh", "-c", findCmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil // ディレクトリがない場合は空扱い
	}

	var containers []Container
	paths := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// 各ファイルをcatしてパース
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

// Stop は指定されたIDのコンテナを停止します。
func (b *WSLBackend) Stop(id string) error {
	fmt.Printf("Stopping container %s...\n", id)

	// 1. 具体的かつ安全な終了: container-shim とその引数の ID を指定
	stopCmd := fmt.Sprintf("pkill -f 'container-shim.*%s'", id)
	_ = b.wslClient.RunDistroCommand("sh", "-c", stopCmd)

	// 2. ステータスを Exited に更新
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	configPath := fmt.Sprintf("%s/config.json", containerDir)

	// WSL内の config.json を読み取ってパース
	cmd := exec.Command("wsl.exe", "-d", b.wslClient.DistroName, "--", "cat", configPath)
	out, err := cmd.Output()
	if err == nil {
		var meta Container
		if err := json.Unmarshal(out, &meta); err == nil {
			meta.Status = "Exited"
			metaJSON, _ := json.Marshal(meta)
			_ = b.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s", configPath))
		}
	}

	fmt.Printf("Container %s stopped.\n", id)
	return nil
}

// Logs はコンテナの出力ログを取得します。
func (b *WSLBackend) Logs(id string) (string, error) {
	logFile := fmt.Sprintf("/var/lib/pocketlinx/containers/%s/console.log", id)

	// WSL 経由でログファイルを読み取る
	cmd := exec.Command("wsl.exe", "-d", b.wslClient.DistroName, "--", "cat", logFile)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read logs for container %s (maybe no logs yet): %w", id, err)
	}

	return string(out), nil
}

func (b *WSLBackend) Remove(id string) error {
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	return b.wslClient.RunDistroCommand("rm", "-rf", containerDir)
}

func (b *WSLBackend) Build(ctxDir string, tag string) (string, error) {
	dockerfilePath := filepath.Join(ctxDir, "Dockerfile")
	df, err := ParseDockerfile(dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	// 1. Calculate Hash Chain to determine where to resume
	// parentHash starts with the Base Image + "FROM"
	parentHash := fmt.Sprintf("%x", sha256.Sum256([]byte("FROM "+df.Base)))

	// Hashes for each instruction index
	stepHashes := make([]string, len(df.Instructions))

	for i, instr := range df.Instructions {
		h, err := CalculateInstructionHash(parentHash, instr, ctxDir)
		if err != nil {
			return "", fmt.Errorf("failed to calculate hash for step %d: %w", i, err)
		}
		stepHashes[i] = h
		parentHash = h
	}

	// 2. Find the last cache hit (Fast Forward)
	lastHitIndex := -1
	for i := len(stepHashes) - 1; i >= 0; i-- {
		cacheFile := path.Join(GetWslCacheDir(), stepHashes[i]+".tar.gz")
		if err := b.wslClient.RunDistroCommand("test", "-f", cacheFile); err == nil {
			lastHitIndex = i
			break
		}
	}

	// 3. Prepare Build Directory
	imageName := tag
	if imageName == "" {
		imageName = strings.ToLower(filepath.Base(ctxDir))
	}
	fmt.Printf("Building image '%s' from %s...\n", imageName, df.Base)

	buildId := fmt.Sprintf("build-%d", os.Getpid())
	buildDir := fmt.Sprintf("/var/lib/pocketlinx/builds/%s", buildId)
	rootfsDir := path.Join(buildDir, "rootfs")
	b.wslClient.RunDistroCommand("mkdir", "-p", rootfsDir)
	defer b.wslClient.RunDistroCommand("rm", "-rf", buildDir)

	// 4. Restore state OR Initialize Base
	if lastHitIndex >= 0 {
		hitHash := stepHashes[lastHitIndex]
		fmt.Printf("CACHED: Resuming from step %d (Hash: %s)\n", lastHitIndex+1, hitHash[:12])
		if _, err := b.LoadCache(hitHash, rootfsDir); err != nil {
			return "", fmt.Errorf("failed to load cache %s: %w", hitHash, err)
		}
	} else {
		// Initialize from Base Image
		baseTarWsl := path.Join(GetWslImagesDir(), df.Base+".tar.gz")
		if err := b.wslClient.RunDistroCommand("test", "-f", baseTarWsl); err != nil {
			fmt.Printf("Base image not found, pulling %s...\n", df.Base)
			if err := b.Pull(df.Base); err != nil {
				return "", fmt.Errorf("failed to pull base image %s: %w", df.Base, err)
			}
		}
		if err := b.wslClient.RunDistroCommand("tar", "-xf", baseTarWsl, "-C", rootfsDir); err != nil {
			return "", fmt.Errorf("failed to extract base image: %w", err)
		}
	}

	// 5. Execute Remaining Steps
	currentWorkdir := "/"
	envPrefix := "" // Need to reconstruct ENV from scratch or cache?
	// PROBLEM: Reconstruct ENV if we skipped steps!
	// We must iterate ALL instructions to build up ENV, even if we skip execution.

	for i, instr := range df.Instructions {
		// Always process ENV state
		if instr.Type == "ENV" {
			for j := 0; j < len(instr.Args); j += 2 {
				k := instr.Args[j]
				v := ""
				if j+1 < len(instr.Args) {
					v = instr.Args[j+1]
				}
				envPrefix += fmt.Sprintf("export %s=%q; ", k, v)
			}
		}

		// Skip execution if covered by cache
		if i <= lastHitIndex {
			// If it was WORKDIR, we need to update currentWorkdir tracking?
			// Actually WORKDIR doesn't persist in FS in a way that affects *path strings* in our Go code,
			// but it affects where `Run` executes.
			// But since we restore `rootfs`, the directories Created by WORKDIR exist.
			// However `currentWorkdir` variable needs to be updated.
			if instr.Type == "WORKDIR" && len(instr.Args) > 0 {
				currentWorkdir = instr.Args[0]
			}
			continue
		}

		// Execute Step
		fmt.Printf("[%d/%d] %s %s\n", i+1, len(df.Instructions), instr.Type, instr.Raw)

		switch instr.Type {
		// ENV is already handled above for prefix string
		case "ENV":
			fmt.Println("Applying ENV...")
			// No FS change usually, but we treat it as a step that creates a layer
			// so that subsequent steps have a distinct parent hash.
			// To be consistent with "Layer Caching":
			// If we modify ENV, we save the current state as a new layer.

		case "RUN":
			if err := b.executeBuildRun(envPrefix, instr.Raw, rootfsDir, currentWorkdir); err != nil {
				return "", fmt.Errorf("RUN failed: %w", err)
			}

		case "COPY":
			if err := b.executeBuildCopy(ctxDir, instr.Args[0], instr.Args[1], rootfsDir, currentWorkdir); err != nil {
				return "", fmt.Errorf("COPY failed: %w", err)
			}

		case "WORKDIR":
			if len(instr.Args) > 0 {
				currentWorkdir = instr.Args[0]
				workdirPath := path.Join(rootfsDir, strings.TrimPrefix(currentWorkdir, "/"))
				b.wslClient.RunDistroCommand("mkdir", "-p", workdirPath)
			}
		}

		// Save Cache after execution
		stepHash := stepHashes[i]
		if err := b.SaveCache(stepHash, rootfsDir); err != nil {
			fmt.Printf("Warning: Failed to save cache for step %d: %v\n", i, err)
		}
	}

	// 6. Final Save
	outputTarWsl := path.Join(GetWslImagesDir(), imageName+".tar.gz")

	// Ideally we just symlink the last cache layer to the image name?
	// or Copy it. Copy is safer.
	// Actually we have the rootfs fully built.
	fmt.Printf("Saving image to %s (WSL)...\n", outputTarWsl)
	if err := b.wslClient.RunDistroCommand("tar", "-czf", outputTarWsl, "-C", rootfsDir, "."); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	fmt.Printf("Successfully built image '%s'\n", imageName)
	return imageName, nil
}

// Volume Management

func GetWslVolumesDir() string {
	return "/var/lib/pocketlinx/volumes"
}

func (b *WSLBackend) CreateVolume(name string) error {
	volDir := path.Join(GetWslVolumesDir(), name)
	// Check if already exists
	if err := b.wslClient.RunDistroCommand("test", "-d", volDir); err == nil {
		return fmt.Errorf("volume '%s' already exists", name)
	}
	return b.wslClient.RunDistroCommand("mkdir", "-p", volDir)
}

func (b *WSLBackend) RemoveVolume(name string) error {
	volDir := path.Join(GetWslVolumesDir(), name)
	// Check before remove
	if err := b.wslClient.RunDistroCommand("test", "-d", volDir); err != nil {
		return fmt.Errorf("volume '%s' not found", name)
	}
	return b.wslClient.RunDistroCommand("rm", "-rf", volDir)
}

func (b *WSLBackend) ListVolumes() ([]string, error) {
	volBase := GetWslVolumesDir()
	// Ensure base dir exists
	b.wslClient.RunDistroCommand("mkdir", "-p", volBase)

	// List directories in volumes dir
	cmd := exec.Command("wsl", "-d", b.wslClient.DistroName, "ls", "-1", volBase)
	output, err := cmd.Output()
	if err != nil {
		// If ls fails (e.g. empty? no, ls on empty dir is fine usually)
		// But if dir doesn't exist it fails.
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var volumes []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			volumes = append(volumes, line)
		}
	}
	return volumes, nil
}

func downloadFile(url string, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (b *WSLBackend) executeBuildRun(envPrefix string, runCmd string, rootfsDir string, currentWorkdir string) error {
	fmt.Printf("STEP: RUN %s\n", runCmd)

	// 3. Create a temporary script for the command to avoid quoting issues
	scriptName := fmt.Sprintf("build_step_%d.sh", time.Now().UnixNano())

	// Use ToSlash to ensure Linux-style paths for WSL commands
	tmpDir := filepath.ToSlash(filepath.Join(rootfsDir, "tmp"))
	scriptDstPath := filepath.ToSlash(filepath.Join(tmpDir, scriptName))

	// Ensure tmp exists
	if err := b.wslClient.RunDistroCommand("mkdir", "-p", tmpDir); err != nil {
		return fmt.Errorf("failed to creating tmp dir: %w", err)
	}

	// Write the script
	scriptContent := fmt.Sprintf("#!/bin/sh\nset -e\n%s\n%s", envPrefix, runCmd)

	// Write using cat
	cmd := exec.Command("wsl", "-d", b.wslClient.DistroName, "sh", "-c", fmt.Sprintf("cat > %s", scriptDstPath))
	cmd.Stdin = strings.NewReader(scriptContent)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write build script: %w", err)
	}

	// Make executable
	if err := b.wslClient.RunDistroCommand("chmod", "+x", scriptDstPath); err != nil {
		return fmt.Errorf("failed to chmod script: %w", err)
	}

	// 隔離環境のセットアップと実行
	fullCmd := fmt.Sprintf(
		"mkdir -p %s/proc %s/sys %s/dev %s%s && "+
			"mount -t proc proc %s/proc && "+
			"mount -t sysfs sys %s/sys && "+
			"rm -f %s/dev/null %s/dev/zero %s/dev/random %s/dev/urandom && "+
			"mknod -m 666 %s/dev/null c 1 3 && "+
			"mknod -m 666 %s/dev/zero c 1 5 && "+
			"mknod -m 666 %s/dev/random c 1 8 && "+
			"mknod -m 666 %s/dev/urandom c 1 9 && "+
			"mkdir -p %s/etc && "+
			"cat /etc/resolv.conf > %s/etc/resolv.conf && "+
			"chroot %s /tmp/%s; "+
			"RET=$?; umount %s/proc %s/sys; exit $RET",
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, currentWorkdir,
		rootfsDir, rootfsDir,
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, // for rm -f
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, // for mknod
		rootfsDir, // for mkdir -p %s/etc
		rootfsDir, // for cat /etc/resolv.conf > ...
		rootfsDir, scriptName,
		rootfsDir, rootfsDir,
	)

	return b.wslClient.RunDistroCommand("unshare", "--mount", "sh", "-c", fullCmd)
}

func (b *WSLBackend) executeBuildCopy(ctxDir, srcArg, destArg, rootfsDir, currentWorkdir string) error {
	src := filepath.Join(ctxDir, srcArg)
	dest := path.Join(rootfsDir, strings.TrimPrefix(path.Join(currentWorkdir, destArg), "/"))
	fmt.Printf("STEP: COPY %s to %s\n", srcArg, destArg)

	srcWsl, err := wsl.WindowsToWslPath(src)
	if err != nil {
		return fmt.Errorf("failed to convert src path: %w", err)
	}

	// ホストからrootfs内へコピー
	// mkdir -p で親ディレクトリを作成し、cp -r で再帰的にコピー
	cmd := fmt.Sprintf("mkdir -p $(dirname %s) && cp -r %s %s", dest, srcWsl, dest)
	return b.wslClient.RunDistroCommand("sh", "-c", cmd)
}

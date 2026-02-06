package container

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"encoding/json"
	"io"
	"net/http"
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

	targetFile := filepath.Join(GetImagesDir(), image+".tar.gz")
	if _, err := os.Stat(targetFile); err == nil {
		fmt.Printf("Image '%s' already exists.\n", image)
		return nil
	}

	fmt.Printf("Pulling image '%s' from %s...\n", image, url)
	// WSL 経由ではなく、ホストの PowerShell を直接使ってダウンロードする
	cmd := exec.Command("powershell.exe", "-Command", fmt.Sprintf("Invoke-WebRequest -Uri %s -OutFile %s", url, targetFile))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error pulling image: %w", err)
	}

	// If it's the system default (alpine), we might want to import it as the system distro
	if image == "alpine" {
		installDir := GetDistroDir()
		absInstallDir, _ := filepath.Abs(installDir)
		absRootfsFile, _ := filepath.Abs(targetFile)

		fmt.Printf("Importing system distro '%s'...\n", DistroName)
		b.wslClient.Run("--unregister", DistroName)
		os.RemoveAll(absInstallDir)
		os.MkdirAll(absInstallDir, 0755)

		err := b.wslClient.Run("--import", DistroName, absInstallDir, absRootfsFile, "--version", "2")
		if err != nil {
			return fmt.Errorf("error importing system distro: %w", err)
		}

		// Install shim
		fmt.Println("Installing container-shim...")
		return b.wslClient.RunDistroCommandWithInput(
			shim.Content,
			"sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim",
		)
	}

	return nil
}

func (b *WSLBackend) Images() ([]string, error) {
	files, err := os.ReadDir(GetImagesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var images []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".tar.gz") {
			name := strings.TrimSuffix(f.Name(), ".tar.gz")
			images = append(images, name)
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

	imageFile := filepath.Join(GetImagesDir(), image+".tar.gz")
	if _, err := os.Stat(imageFile); os.IsNotExist(err) {
		return fmt.Errorf("image '%s' not found. Please run 'plx pull %s' first", image, image)
	}

	wslRootfsPath, err := wsl.WindowsToWslPath(imageFile)
	if err != nil {
		return fmt.Errorf("path resolving error: %w", err)
	}

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
			srcWsl, err := wsl.WindowsToWslPath(m.Source)
			if err != nil {
				fmt.Printf("Warning: Failed to convert mount path %s: %v\n", m.Source, err)
				continue
			}
			mParts = append(mParts, fmt.Sprintf("%s:%s", srcWsl, m.Target))
		}
		if len(mParts) > 0 {
			mountsStr = strings.Join(mParts, ",")
		}
	}

	// 3. Metadata
	meta := Container{
		ID:      containerId,
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

func (b *WSLBackend) Build(ctxDir string) (string, error) {
	dockerfilePath := filepath.Join(ctxDir, "Dockerfile")
	df, err := ParseDockerfile(dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	// 1. ベースイメージの準備
	imageName := strings.ToLower(filepath.Base(ctxDir))
	fmt.Printf("Building image '%s' from %s...\n", imageName, df.Base)

	if err := b.Pull(df.Base); err != nil {
		return "", fmt.Errorf("failed to pull base image %s: %w", df.Base, err)
	}

	// ビルドディレクトリの作成 (WSL内)
	buildId := fmt.Sprintf("build-%d", os.Getpid())
	buildDir := fmt.Sprintf("/var/lib/pocketlinx/builds/%s", buildId)
	rootfsDir := path.Join(buildDir, "rootfs")
	b.wslClient.RunDistroCommand("mkdir", "-p", rootfsDir)
	defer b.wslClient.RunDistroCommand("rm", "-rf", buildDir) // 終了時に掃除

	// ベースイメージの展開
	baseTar := filepath.Join(GetImagesDir(), df.Base+".tar.gz")
	wslBaseTar, _ := wsl.WindowsToWslPath(baseTar)
	fmt.Println("Extracting base image...")
	if err := b.wslClient.RunDistroCommand("tar", "-xf", wslBaseTar, "-C", rootfsDir); err != nil {
		return "", fmt.Errorf("failed to extract base image: %w", err)
	}

	currentWorkdir := "/"
	if df.Workdir != "" {
		currentWorkdir = df.Workdir
	}

	// 2. 構築ステップの実行
	// 環境変数の準備
	envPrefix := ""
	for k, v := range df.Env {
		envPrefix += fmt.Sprintf("export %s=%q; ", k, v)
	}

	for _, runCmd := range df.Run {
		fmt.Printf("STEP: RUN %s\n", runCmd)

		// 隔離環境のセットアップと実行
		// mount /proc, /sys, /dev, cp resolv.conf, then chroot
		// 最後に念のため umount する（unshare --mount なので終了時に消えるはずだが明示的に）
		fullCmd := fmt.Sprintf(
			"mkdir -p %s/proc %s/sys %s/dev %s%s && "+
				"mount -t proc proc %s/proc && "+
				"mount -t sysfs sys %s/sys && "+
				"mknod -m 666 %s/dev/null c 1 3 && "+
				"mknod -m 666 %s/dev/zero c 1 5 && "+
				"mknod -m 666 %s/dev/random c 1 8 && "+
				"mknod -m 666 %s/dev/urandom c 1 9 && "+
				"mkdir -p %s/etc && "+
				"cat /etc/resolv.conf > %s/etc/resolv.conf && "+
				"chroot %s sh -c 'cd %s && %s %s'; "+
				"RET=$?; umount %s/proc %s/sys; exit $RET",
			rootfsDir, rootfsDir, rootfsDir, rootfsDir, currentWorkdir,
			rootfsDir, rootfsDir,
			rootfsDir, rootfsDir, rootfsDir, rootfsDir, // for mknod
			rootfsDir, // for mkdir -p %s/etc
			rootfsDir, // for cat /etc/resolv.conf > ...
			rootfsDir,
			currentWorkdir, envPrefix, runCmd,
			rootfsDir, rootfsDir,
		)

		err := b.wslClient.RunDistroCommand("unshare", "--mount", "sh", "-c", fullCmd)
		if err != nil {
			return "", fmt.Errorf("RUN failed: %w", err)
		}
	}

	// 3. COPY命令の実行
	for _, cp := range df.Copy {
		src := filepath.Join(ctxDir, cp[0])
		dest := path.Join(rootfsDir, strings.TrimPrefix(path.Join(currentWorkdir, cp[1]), "/"))
		fmt.Printf("STEP: COPY %s to %s\n", cp[0], cp[1])

		srcWsl, _ := wsl.WindowsToWslPath(src)
		// ホストからrootfs内へコピー
		if err := b.wslClient.RunDistroCommand("sh", "-c", fmt.Sprintf("mkdir -p $(dirname %s) && cp -r %s %s", dest, srcWsl, dest)); err != nil {
			return "", fmt.Errorf("COPY failed: %w", err)
		}
	}

	// 4. 新イメージの保存
	outputTar := filepath.Join(GetImagesDir(), imageName+".tar.gz")
	wslOutputTar, _ := wsl.WindowsToWslPath(outputTar)
	fmt.Printf("Saving image to %s...\n", outputTar)

	if err := b.wslClient.RunDistroCommand("tar", "-czf", wslOutputTar, "-C", rootfsDir, "."); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	fmt.Printf("Successfully built image '%s'\n", imageName)
	return imageName, nil
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

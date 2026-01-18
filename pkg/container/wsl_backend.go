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

	// 1. Create necessary directories
	_ = GetImagesDir()
	_ = GetDistroDir()
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

	// Get original command as a string
	var quotedArgs []string
	for _, arg := range opts.Args {
		quotedArgs = append(quotedArgs, fmt.Sprintf("%q", arg))
	}
	userCmd := strings.Join(quotedArgs, " ")

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

	// 4. Execution via shim
	shimCmd := fmt.Sprintf("/bin/sh /usr/local/bin/container-shim %s %s %s", rootfsDir, mountsStr, userCmd)

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

	unshareArgs := []string{
		"unshare", "--mount", "--pid", "--fork", "--uts",
		"sh", "-c", portCmd + shimCmd,
	}

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

	err = b.wslClient.RunDistroCommand(unshareArgs...)

	// 終了後のステータス更新
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

func (b *WSLBackend) Remove(id string) error {
	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", id)
	return b.wslClient.RunDistroCommand("rm", "-rf", containerDir)
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

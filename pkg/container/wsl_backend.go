package container

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"encoding/json"
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

func (b *WSLBackend) Setup() error {
	fmt.Println("Setting up PocketLinx environment (WSL2)...")

	// 1. Download rootfs
	if _, err := os.Stat(RootfsFile); os.IsNotExist(err) {
		fmt.Printf("Downloading Alpine rootfs from %s...\n", RootfsUrl)
		// wslClient.Run は内部で wsl.exe を呼ぶので、コマンドライン引数として渡す
		err := b.wslClient.Run("powershell", "-Command", fmt.Sprintf("Invoke-WebRequest -Uri %s -OutFile %s", RootfsUrl, RootfsFile))
		if err != nil {
			return fmt.Errorf("error downloading rootfs: %w", err)
		}
	}

	// 2. Import to WSL
	installDir := "distro"
	absInstallDir, _ := filepath.Abs(installDir)
	absRootfsFile, _ := filepath.Abs(RootfsFile)

	fmt.Printf("Importing %s into WSL as '%s'...\n", absRootfsFile, DistroName)

	// Clean up existing distro
	b.wslClient.Run("--unregister", DistroName)

	os.RemoveAll(absInstallDir)
	if err := os.MkdirAll(absInstallDir, 0755); err != nil {
		return fmt.Errorf("error creating install directory: %w", err)
	}

	err := b.wslClient.Run("--import", DistroName, absInstallDir, absRootfsFile, "--version", "2")
	if err != nil {
		os.RemoveAll(absInstallDir)
		return fmt.Errorf("error importing distro: %w", err)
	}

	// 3. Create container-shim
	fmt.Println("Installing container-shim...")
	err = b.wslClient.RunDistroCommandWithInput(
		shim.Content,
		"sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim",
	)
	if err != nil {
		return fmt.Errorf("error installing shim: %w", err)
	}

	return nil
}

func (b *WSLBackend) Run(args []string) error {
	containerId := fmt.Sprintf("c-%d", os.Getpid())
	fmt.Printf("Running %v in container %s (WSL2)...\n", args, containerId)

	containerDir := fmt.Sprintf("/var/lib/pocketlinx/containers/%s", containerId)
	rootfsDir := path.Join(containerDir, "rootfs")

	// Get original command as a string
	userCmd := ""
	for _, arg := range args {
		userCmd += fmt.Sprintf(" %q", arg)
	}

	wslRootfsPath, err := wsl.WindowsToWslPath(RootfsFile)
	if err != nil {
		return fmt.Errorf("path resolving error: %w", err)
	}

	// Provisioning
	// 明示的にコンテナディレクトリ（親）を作成してからrootfsを作成する
	setupCmd := fmt.Sprintf("mkdir -p %s && mkdir -p %s && tar -xf %s -C %s", containerDir, rootfsDir, wslRootfsPath, rootfsDir)
	if err := b.wslClient.RunDistroCommand("sh", "-c", setupCmd); err != nil {
		return fmt.Errorf("provisioning failed (path: %s): %w", containerDir, err)
	}

	// メタデータの作成と保存（ディレクトリ作成後に行う）
	meta := Container{
		ID:      containerId,
		Command: userCmd,
		Created: time.Now(),
		Status:  "Running",
	}
	metaJSON, _ := json.Marshal(meta)
	b.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > %s/config.json", containerDir))

	shimCmd := fmt.Sprintf("/bin/sh /usr/local/bin/container-shim %s %s", rootfsDir, userCmd)

	unshareArgs := []string{
		"unshare", "--pid", "--fork", "--mount-proc", "--mount", "--uts",
		"sh", "-c", shimCmd,
	}

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

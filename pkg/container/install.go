package container

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// InstallBinary は現在実行中のバイナリを共通ディレクトリに配置し、PATH を通します。
func InstallBinary() error {
	// 1. インストール先を決定 (%APPDATA%\PocketLinx\bin)
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return fmt.Errorf("could not find APPDATA environment variable")
	}
	installDir := filepath.Join(appData, "PocketLinx", "bin")
	installPath := filepath.Join(installDir, "plx.exe")

	fmt.Printf("Installing PocketLinx to %s...\n", installDir)

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	// 2. 現在のバイナリを取得
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %w", err)
	}

	// 3. コピー (実行中のファイルを直接上書きできない場合があるため一度試行)
	if err := copyFile(selfPath, installPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// 4. PATH 環境変数を更新 (Windows ユーザー環境変数)
	// PowerShell を使ってレジストリを介さずスマートに更新
	fmt.Println("Adding PocketLinx to your PATH...")
	psCmd := fmt.Sprintf(`
		$oldPath = [Environment]::GetEnvironmentVariable("Path", "User")
		if ($oldPath -notlike "*%s*") {
			$newPath = "$oldPath;%s"
			[Environment]::SetEnvironmentVariable("Path", $newPath, "User")
			Write-Host "PATH updated successfully."
		} else {
			Write-Host "PATH already contains PocketLinx."
		}
	`, installDir, installDir)

	cmd := exec.Command("powershell", "-Command", psCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update PATH: %s", string(out))
	}

	fmt.Println("\nInstallation successful!")
	fmt.Println("Please restart your terminal to use 'plx' from anywhere.")
	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

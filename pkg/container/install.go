package container

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"PocketLinx/pkg/version"
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

	// バージョン情報を表示 (v1.0.4 - UX)
	fmt.Printf("Current executable version: %s\n", version.Current)
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

	// 4. 競合バイナリの検知とクリーンアップ (v1.0.5)
	cleanupConflicts(installPath)

	// 5. PATH 環境変数を更新 (Windows ユーザー環境変数)
	// 公式パスを PATH の先頭に配置 (v1.0.5: Priority Update)
	fmt.Println("Ensuring PocketLinx has top priority in your PATH...")
	psCmd := fmt.Sprintf(`
		$officialPath = "%s"
		$oldPath = [Environment]::GetEnvironmentVariable("Path", "User")
		
		# 分割して現在のリストを取得
		$pathList = $oldPath -split ";" | Where-Object { $_ -ne "" }
		
		# 公式パスを除去（すでにある場合も一度抜く）
		$newPathList = $pathList | Where-Object { $_ -ne $officialPath }
		
		# 先頭に公式パスを挿入
		$finalPath = "$officialPath;" + ($newPathList -join ";")
		
		[Environment]::SetEnvironmentVariable("Path", $finalPath, "User")
		Write-Host "PATH priority updated successfully."
	`, installDir)

	cmd := exec.Command("powershell", "-Command", psCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update PATH: %s", string(out))
	}

	fmt.Println("\nInstallation successful!")
	fmt.Println("Please restart your terminal to ensure 'plx' refers to the latest version.")
	return nil
}

func cleanupConflicts(officialPath string) {
	fmt.Println("\nChecking for conflicting 'plx.exe' on your PATH...")

	// where.exe を使用してすべての plx.exe を取得
	out, err := exec.Command("where.exe", "plx.exe").Output()
	if err != nil {
		return // plx.exe が他に見つからない場合は正常終了
	}

	conflicts := strings.Split(strings.TrimSpace(string(out)), "\r\n")
	reader := bufio.NewReader(os.Stdin)

	absOfficial, _ := filepath.Abs(officialPath)

	for _, p := range conflicts {
		absP, err := filepath.Abs(p)
		if err != nil || strings.EqualFold(absP, absOfficial) {
			continue // 公式パスまたはエラーはスキップ
		}

		fmt.Printf("\n[Conflict] Found another 'plx.exe' at: %s\n", p)
		fmt.Printf("Do you want to DELETE this old version? [y/N]: ")

		line, _ := reader.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))

		if answer == "y" || answer == "yes" {
			if err := os.Remove(absP); err != nil {
				fmt.Printf("Error: Could not delete %s. It might be in use.\n", p)
				// 失敗した場合はリネームを試行
				renameToBackup(absP)
			} else {
				fmt.Printf("Successfully deleted: %s\n", p)
			}
		} else {
			// YES 以外はリネームして無効化
			renameToBackup(absP)
		}
	}
}

func renameToBackup(path string) {
	backupPath := path + ".legacy_backup"
	if err := os.Rename(path, backupPath); err != nil {
		fmt.Printf("Warning: Could not disable %s. Please manually remove it.\n", path)
	} else {
		fmt.Printf("Disabled (renamed to .legacy_backup): %s\n", path)
	}
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

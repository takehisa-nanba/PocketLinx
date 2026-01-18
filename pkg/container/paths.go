package container

import (
	"os"
	"path/filepath"
)

// GetDataDir は PocketLinx のデータ保存ディレクトリを返します。
// デフォルトは %USERPROFILE%\.pocketlinx です。
func GetDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// ホームディレクトリが取得できない場合はカレントディレクトリフォールバック
		return ".pocketlinx"
	}
	dir := filepath.Join(home, ".pocketlinx")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetImagesDir はイメージ保存ディレクトリを返します。
func GetImagesDir() string {
	dir := filepath.Join(GetDataDir(), "images")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetDistroDir はシステムディストリビューション保存ディレクトリを返します。
func GetDistroDir() string {
	dir := filepath.Join(GetDataDir(), "distro")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

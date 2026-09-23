package wslbridge

import (
	"os"
	"path/filepath"
)

func systemWSL() (string, error) {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "wsl.exe"), nil
}

package container

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ProjectConfig は plx.json の構造を定義します。
type ProjectConfig struct {
	Image   string   `json:"image"`
	Mounts  []Mount  `json:"mounts"`
	Command []string `json:"command"`
	Workdir string   `json:"workdir"`
}

// LoadProjectConfig はカレントディレクトリから plx.json を読み込みます。
func LoadProjectConfig() (*ProjectConfig, error) {
	data, err := os.ReadFile("plx.json")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // ファイルがないのは正常
		}
		return nil, err
	}

	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadProjectConfigFromDir reads plx.json from a specific directory.
func LoadProjectConfigFromDir(dir string) (*ProjectConfig, error) {
	filePath := filepath.Join(dir, "plx.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

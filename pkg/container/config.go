package container

import (
	"encoding/json"
	"os"
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

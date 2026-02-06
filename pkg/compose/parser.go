package compose

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseComposeFile reads a yaml file and returns the config
func ParseComposeFile(path string) (*ComposeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ComposeConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Basic validation
	if len(config.Services) == 0 {
		return nil, fmt.Errorf("no services defined in compose file")
	}

	return &config, nil
}

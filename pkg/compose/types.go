package compose

// ComposeConfig represents the structure of plx-compose.yml
type ComposeConfig struct {
	Version  string                   `yaml:"version"`
	Services map[string]ServiceConfig `yaml:"services"`
	Volumes  map[string]interface{}   `yaml:"volumes,omitempty"`
}

// ServiceConfig represents a single service definition
type ServiceConfig struct {
	Image       string   `yaml:"image"`
	Command     []string `yaml:"command,omitempty"` // Supports ["cmd", "arg"] format
	Ports       []string `yaml:"ports,omitempty"`   // "8080:80"
	Environment []string `yaml:"environment,omitempty"`
	Volumes     []string `yaml:"volumes,omitempty"`
	WorkingDir  string   `yaml:"working_dir,omitempty"`
	Restart     string   `yaml:"restart,omitempty"`
}

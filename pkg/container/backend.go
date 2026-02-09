package container

import "time"

// Container はコンテナの情報を保持する構造体です。
type Container struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Image   string        `json:"image"`
	Command string        `json:"command"`
	Created time.Time     `json:"created"`
	Status  string        `json:"status"`
	Ports   []PortMapping `json:"ports"`
	IP      string        `json:"ip"`
	Config  RunOptions    `json:"config"`
}

// Mount はホストパスとコンテナパスのペアを表します。
type Mount struct {
	Source string
	Target string
}

// PortMapping はホストとコンテナのポート対応を表します。
type PortMapping struct {
	Host      int
	Container int
}

// ImageMetadata stores the default runtime configuration for an image
type ImageMetadata struct {
	User    string            `json:"user"`
	Workdir string            `json:"workdir"`
	Env     map[string]string `json:"env"`
	Command []string          `json:"command"`
}

// RunOptions はコンテナ実行時の詳細設定を保持する構造体です。
type RunOptions struct {
	Image       string
	Name        string
	Args        []string
	Mounts      []Mount
	Env         map[string]string
	Ports       []PortMapping
	Interactive bool
	Detach      bool
	User        string
	Workdir     string
	ExtraHosts  []string // List of "hostname:ip" mappings
}

// Backend はコンテナ実行の基盤（WSL2, Linux Native等）を抽象化するインターフェースです。
type Backend interface {
	Setup() error
	Install() error
	Pull(image string) error
	Images() ([]string, error)
	Run(opts RunOptions) error
	Start(id string) error
	List() ([]Container, error)
	Stop(id string) error
	Logs(id string) (string, error)
	Remove(id string) error
	Build(ctxDir string, dockerfile string, tag string) (string, error) // Dockerfileからビルドしてイメージ名を返す
	Prune() error
	Diff(image1, image2 string) (string, error)
	ExportDiff(baseImage, targetImage, outputPath string) error

	// Volume Management
	CreateVolume(name string) error
	RemoveVolume(name string) error
	ListVolumes() ([]string, error)

	GetIP(id string) (string, error)
	Update(id string, opts RunOptions) error
	Exec(id string, cmd []string, interactive bool) error
}

// Dockerfile represents the parsed content of a Dockerfile
type Dockerfile struct {
	Base         string
	Instructions []Instruction
}

// Instruction represents a single step in the Dockerfile
type Instruction struct {
	Type string   // "RUN", "COPY", "ENV", "WORKDIR", "CMD", "EXPOSE"
	Args []string // For COPY: [src, dest], For ENV: [key, value]
	Raw  string   // Original command string (useful for RUN)
}

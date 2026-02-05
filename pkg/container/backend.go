package container

import "time"

// Container はコンテナの情報を保持する構造体です。
type Container struct {
	ID      string    `json:"id"`
	Command string    `json:"command"`
	Created time.Time `json:"created"`
	Status  string    `json:"status"`
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

// RunOptions はコンテナ実行時の詳細設定を保持する構造体です。
type RunOptions struct {
	Image       string
	Args        []string
	Mounts      []Mount
	Env         map[string]string
	Ports       []PortMapping
	Interactive bool
	Detach      bool
}

// Backend はコンテナ実行の基盤（WSL2, Linux Native等）を抽象化するインターフェースです。
type Backend interface {
	Setup() error
	Install() error
	Pull(image string) error
	Images() ([]string, error)
	Run(opts RunOptions) error
	List() ([]Container, error)
	Stop(id string) error
	Logs(id string) (string, error)
	Remove(id string) error
	Build(ctxDir string) (string, error) // Dockerfileからビルドしてイメージ名を返す
}

// Dockerfile represents the parsed content of a Dockerfile
type Dockerfile struct {
	Base    string
	Run     []string
	Env     map[string]string
	Expose  []int
	Cmd     []string
	Workdir string
	Copy    [][2]string // [source, dest]
}

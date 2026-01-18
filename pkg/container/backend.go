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

// RunOptions はコンテナ実行時の詳細設定を保持する構造体です。
type RunOptions struct {
	Args   []string
	Mounts []Mount
}

// Backend はコンテナ実行の基盤（WSL2, Linux Native等）を抽象化するインターフェースです。
type Backend interface {
	Setup() error
	Run(opts RunOptions) error
	List() ([]Container, error)
	Remove(id string) error
}

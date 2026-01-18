package container

import "time"

// Container はコンテナの情報を保持する構造体です。
type Container struct {
	ID      string    `json:"id"`
	Command string    `json:"command"`
	Created time.Time `json:"created"`
	Status  string    `json:"status"`
}

// Backend はコンテナ実行の基盤（WSL2, Linux Native等）を抽象化するインターフェースです。
type Backend interface {
	Setup() error
	Run(args []string) error
	List() ([]Container, error)
	Remove(id string) error
}

// RunOptions は将来的にコンテナ実行時の詳細設定（メモリ制限等）を追加するための構造体です。
type RunOptions struct {
	// 将来的に追加: MemoryLimit, CPUQuota, EnvVars, etc.
}

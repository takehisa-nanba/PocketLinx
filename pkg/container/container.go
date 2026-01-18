package container

const (
	DistroName = "pocketlinx"
	RootfsUrl  = "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz"
	RootfsFile = "alpine-rootfs.tar.gz"
)

// Engine はコンテナのライフサイクルを管理します。
type Engine struct {
	backend Backend
}

// NewEngine は新しいコンテナエンジンを作成します。
func NewEngine(backend Backend) *Engine {
	return &Engine{
		backend: backend,
	}
}

// Setup は開発環境の初期化を行います。
func (e *Engine) Setup() error {
	return e.backend.Setup()
}

// Run はコンテナ内でコマンドを実行します。
func (e *Engine) Run(args []string) error {
	return e.backend.Run(args)
}

// List はコンテナの一覧を取得します。
func (e *Engine) List() ([]Container, error) {
	return e.backend.List()
}

// Remove は指定されたコンテナを削除します。
func (e *Engine) Remove(id string) error {
	return e.backend.Remove(id)
}

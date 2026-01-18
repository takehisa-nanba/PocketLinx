package container

const (
	DistroName = "pocketlinx"
)

var SupportedImages = map[string]string{
	"alpine": "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz",
	"ubuntu": "https://partner-images.canonical.com/core/jammy/current/ubuntu-jammy-core-cloudimg-amd64-root.tar.gz",
}

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
	if err := e.backend.Setup(); err != nil {
		return err
	}
	// デフォルトイメージとして alpine を準備
	return e.backend.Pull("alpine")
}

// Install はバイナリをインストールします。
func (e *Engine) Install() error {
	return e.backend.Install()
}

// Pull はイメージをダウンロードします。
func (e *Engine) Pull(image string) error {
	return e.backend.Pull(image)
}

// Images は利用可能なイメージの一覧を取得します。
func (e *Engine) Images() ([]string, error) {
	return e.backend.Images()
}

// Run はコンテナ内でコマンドを実行します。
func (e *Engine) Run(opts RunOptions) error {
	return e.backend.Run(opts)
}

// List はコンテナの一覧を取得します。
func (e *Engine) List() ([]Container, error) {
	return e.backend.List()
}

// Remove は指定されたコンテナを削除します。
func (e *Engine) Remove(id string) error {
	return e.backend.Remove(id)
}

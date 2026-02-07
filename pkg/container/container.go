package container

const (
	DistroName = "pocketlinx"
)

var SupportedImages = map[string]string{
	"alpine": "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/x86_64/alpine-minirootfs-3.21.0-x86_64.tar.gz",
	"ubuntu": "https://partner-images.canonical.com/core/jammy/current/ubuntu-jammy-core-cloudimg-amd64-root.tar.gz",
}

// RunOptions defined in backend.go

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
	// 1. デフォルトイメージとして alpine を準備 (これにより pocketlinx ディストロが作成される)
	if err := e.backend.Pull("alpine"); err != nil {
		return err
	}
	// 2. ディストロ内部の設定やパッチを適用
	return e.backend.Setup()
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

func (e *Engine) Start(id string) error {
	return e.backend.Start(id)
}

// List はコンテナの一覧を取得します。
func (e *Engine) List() ([]Container, error) {
	return e.backend.List()
}

// Remove は指定されたコンテナを削除します。
func (e *Engine) Remove(id string) error {
	return e.backend.Remove(id)
}

func (e *Engine) Stop(id string) error {
	return e.backend.Stop(id)
}

func (e *Engine) Logs(id string) (string, error) {
	return e.backend.Logs(id)
}

func (e *Engine) Build(ctxDir string, tag string) (string, error) {
	return e.backend.Build(ctxDir, tag)
}

func (e *Engine) Prune() error {
	return e.backend.Prune()
}

func (e *Engine) CreateVolume(name string) error {
	return e.backend.CreateVolume(name)
}

func (e *Engine) RemoveVolume(name string) error {
	return e.backend.RemoveVolume(name)
}

func (e *Engine) ListVolumes() ([]string, error) {
	return e.backend.ListVolumes()
}

func (e *Engine) GetIP(id string) (string, error) {
	// We need to extend Backend interface as well if Engine uses it.
	// But actually Engine talks to Backend.
	return e.backend.GetIP(id)
}

func (e *Engine) Update(id string, opts RunOptions) error {
	return e.backend.Update(id, opts)
}

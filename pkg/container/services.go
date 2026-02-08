package container

// RuntimeService handles container lifecycle operations (execution, process management)
type RuntimeService interface {
	Run(opts RunOptions) error
	Start(id string) error
	Stop(id string) error
	List() ([]Container, error)
	Logs(id string) (string, error)
	Remove(id string) error
	GetIP(id string) (string, error)
	Update(id string, opts RunOptions) error
	Exec(id string, cmd []string, interactive bool) error
}

// ImageService handles image management (pull, build, cache)
type ImageService interface {
	Pull(image string) error
	Build(ctxDir string, tag string) (string, error)
	Images() ([]string, error)
	Prune() error
}

// VolumeService handles persistent storage management
type VolumeService interface {
	Create(name string) error
	Remove(name string) error
	List() ([]string, error)
}

// NetworkService handles container network isolation and connectivity
type NetworkService interface {
	SetupBridge() error
	AllocateIP() (string, error)
	ReleaseIP(ip string) error
	ConnectContainer(id string, ip string) error
}

package container

// RuntimeService handles container lifecycle operations (execution, process management)
type RuntimeService interface {
	Run(opts RunOptions) error
	Stop(id string) error
	List() ([]Container, error)
	Logs(id string) (string, error)
	Remove(id string) error
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

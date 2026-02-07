//go:build linux

package container


import (
	"os"
	"path/filepath"
)

type LinuxVolumeService struct {
	rootDir string
}

func NewLinuxVolumeService(rootDir string) *LinuxVolumeService {
	return &LinuxVolumeService{rootDir: rootDir}
}

func (s *LinuxVolumeService) Create(name string) error {
	return os.MkdirAll(filepath.Join(s.rootDir, "volumes", name), 0755)
}

func (s *LinuxVolumeService) Remove(name string) error {
	return os.RemoveAll(filepath.Join(s.rootDir, "volumes", name))
}

func (s *LinuxVolumeService) List() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.rootDir, "volumes"))
	if err != nil {
		return []string{}, nil
	}
	var vols []string
	for _, e := range entries {
		if e.IsDir() {
			vols = append(vols, e.Name())
		}
	}
	return vols, nil
}

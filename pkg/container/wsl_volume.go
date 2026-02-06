package container

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"PocketLinx/pkg/wsl"
)

// WSLVolumeService implements VolumeService
type WSLVolumeService struct {
	wslClient *wsl.Client
}

func NewWSLVolumeService(client *wsl.Client) *WSLVolumeService {
	return &WSLVolumeService{wslClient: client}
}

func GetWslVolumesDir() string {
	return "/var/lib/pocketlinx/volumes"
}

func (s *WSLVolumeService) Create(name string) error {
	volDir := path.Join(GetWslVolumesDir(), name)
	// Check if already exists
	if err := s.wslClient.RunDistroCommand("test", "-d", volDir); err == nil {
		return fmt.Errorf("volume '%s' already exists", name)
	}
	return s.wslClient.RunDistroCommand("mkdir", "-p", volDir)
}

func (s *WSLVolumeService) Remove(name string) error {
	volDir := path.Join(GetWslVolumesDir(), name)
	// Check before remove
	if err := s.wslClient.RunDistroCommand("test", "-d", volDir); err != nil {
		return fmt.Errorf("volume '%s' not found", name)
	}
	return s.wslClient.RunDistroCommand("rm", "-rf", volDir)
}

func (s *WSLVolumeService) List() ([]string, error) {
	volBase := GetWslVolumesDir()
	// Ensure base dir exists
	s.wslClient.RunDistroCommand("mkdir", "-p", volBase)

	// List directories in volumes dir
	cmd := exec.Command("wsl", "-d", s.wslClient.DistroName, "ls", "-1", volBase)
	output, err := cmd.Output()
	if err != nil {
		// If ls fails (e.g. empty)
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var volumes []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			volumes = append(volumes, line)
		}
	}
	return volumes, nil
}

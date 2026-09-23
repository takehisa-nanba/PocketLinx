//go:build !linux || !amd64

package environment

import (
	"fmt"
	"os"
)

func supported() error {
	return fmt.Errorf("environment save/restore currently requires Linux/amd64; Windows/WSL integration is not implemented")
}
func lockDirectory(string) (*os.File, error)    { return nil, supported() }
func openRegular(string) (*os.File, error)      { return nil, supported() }
func ownership(os.FileInfo) (uint32, uint32)    { return 0, 0 }
func publishDirectory(string, string) error     { return supported() }
func applyMetadata(string, Entry) error         { return supported() }
func checkAttributes(string, os.FileInfo) error { return supported() }
func rejectMounts(string) error                 { return supported() }

//go:build !linux || !amd64

package envrun

import (
	"context"
	"fmt"
	"os"
)

const HelperCommand = "__plx-env-chroot-helper"

type ExperimentalChroot struct{ Executable string }

func unsupported() error {
	return fmt.Errorf("experimental execution requires Linux/amd64; Windows/WSL adapter is not implemented")
}
func lockDirectory(string) (*os.File, error)                        { return nil, unsupported() }
func (ExperimentalChroot) Run(context.Context, Plan, Streams) error { return unsupported() }
func RunHelper() error                                              { return unsupported() }

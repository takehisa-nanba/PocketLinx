//go:build !linux || !amd64

package main

import (
	"fmt"
	"io"
)

func runBridge([]string, io.Writer) error { return fmt.Errorf("bridge helper requires Linux/amd64") }

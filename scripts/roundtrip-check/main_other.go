//go:build !linux || !amd64

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "roundtrip-check requires Linux/amd64")
	os.Exit(1)
}

//go:build !windows

package wslbridge

import "fmt"

func systemWSL() (string, error) { return "", fmt.Errorf("WSL adapter requires Windows") }

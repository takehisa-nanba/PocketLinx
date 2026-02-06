package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"PocketLinx/pkg/api"
	"PocketLinx/pkg/container"
)

func handleDashboard(engine *container.Engine) {
	port := 3000
	server := api.NewServer(engine)
	fmt.Printf("Starting PocketLinx Dashboard on port %d...\n", port)

	// Windows: 自動でブラウザを開く
	if runtime.GOOS == "windows" {
		go func() {
			exec.Command("cmd", "/c", "start", fmt.Sprintf("http://localhost:%d", port)).Run()
		}()
	}

	if err := server.Start(port); err != nil {
		fmt.Fprintf(os.Stderr, "Dashboard failed: %v\n", err)
		os.Exit(1)
	}
}

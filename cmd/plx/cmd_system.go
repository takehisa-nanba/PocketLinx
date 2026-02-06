package main

import (
	"fmt"
	"os"

	"PocketLinx/pkg/container"
)

func handleSetup(engine *container.Engine) {
	if err := container.CheckRequirements(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: System requirements not met:\n  %v\n", err)
		os.Exit(1)
	}
	if err := engine.Setup(); err != nil {
		fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
		os.Exit(1)
	}
}

func handleInstall(engine *container.Engine) {
	if err := engine.Install(); err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		os.Exit(1)
	}
}

func handleVersion() {
	// TODO: Inject version from LDFlags
	fmt.Println("PocketLinx v0.3.0 (WSL Native Architecture)")
}

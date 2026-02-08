package main

import (
	"fmt"
	"os"

	"PocketLinx/pkg/container"
)

func handlePull(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx pull <image_name>")
		fmt.Println("Supported images: alpine, ubuntu")
		os.Exit(1)
	}
	if err := engine.Pull(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Pull failed: %v\n", err)
		os.Exit(1)
	}
}

func handleImages(engine *container.Engine) {
	images, err := engine.Images()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list images: %v\n", err)
		os.Exit(1)
	}
	headers := []string{"IMAGE NAME"}
	var rows [][]string
	for _, img := range images {
		rows = append(rows, []string{img})
	}
	container.PrintTable(headers, rows)
}

func handleBuild(engine *container.Engine, args []string) {
	ctxDir := "."
	targetImage := ""

	// Parse arguments manually to support -t/--tag
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-t" || arg == "--tag" {
			if i+1 < len(args) {
				targetImage = args[i+1]
				i++ // skip value
			} else {
				fmt.Println("Error: flag needs an argument: -t")
				os.Exit(1)
			}
		} else {
			ctxDir = arg
		}
	}

	// Try to load config to get image name if not provided via flag
	if targetImage == "" {
		config, _ := container.LoadProjectConfigFromDir(ctxDir)
		if config != nil && config.Image != "" {
			targetImage = config.Image
		}
	}

	img, err := engine.Build(ctxDir, targetImage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully built image: %s\n", img)
}

func handlePrune(engine *container.Engine) {
	fmt.Println("Pruning build cache...")
	if err := engine.Prune(); err != nil {
		fmt.Fprintf(os.Stderr, "Prune failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Build cache cleared.")
}

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
	configFile := ""

	// Parse arguments manually to support -t/--tag and -f/--file
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t", "--tag":
			if i+1 < len(args) {
				targetImage = args[i+1]
				i++ // skip value
			} else {
				fmt.Println("Error: flag needs an argument: -t")
				os.Exit(1)
			}
		case "-f", "--file":
			if i+1 < len(args) {
				configFile = args[i+1]
				i++
			} else {
				fmt.Println("Error: flag needs an argument: -f")
				os.Exit(1)
			}
		default:
			ctxDir = args[i]
		}
	}

	// Try to load config to get image name if not provided via flag
	if targetImage == "" {
		config, _ := container.LoadProjectConfigFromDir(ctxDir)
		if config != nil && config.Image != "" {
			targetImage = config.Image
		}
	}

	img, err := engine.Build(ctxDir, configFile, targetImage)
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

func handleDiff(engine *container.Engine, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: plx diff <image1> <image2>")
		os.Exit(1)
	}
	diff, err := engine.Diff(args[0], args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Diff failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(diff)
}

func handlePackage(engine *container.Engine, args []string) {
	if len(args) < 3 {
		fmt.Println("Usage: plx package <base_image> <target_image> <output_path.tar.gz>")
		os.Exit(1)
	}
	base := args[0]
	target := args[1]
	output := args[2]

	if err := engine.ExportDiff(base, target, output); err != nil {
		fmt.Fprintf(os.Stderr, "Package export failed: %v\n", err)
		os.Exit(1)
	}
}

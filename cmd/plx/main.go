package main

import (
	"fmt"
	"os"

	"PocketLinx/pkg/container"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	if cmd == "version" {
		handleVersion()
		return
	}

	var backend container.Backend

	// OS判定とバックエンドの選択（ビルドタグで切り替え）
	backend = container.NewBackend()

	engine := container.NewEngine(backend)

	switch cmd {
	case "setup":
		handleSetup(engine)
	case "install":
		handleInstall(engine)
	case "pull":
		handlePull(engine, args)
	case "images":
		handleImages(engine)
	case "run":
		handleRun(engine, args)
	case "ps":
		handlePs(engine)
	case "stop":
		handleStop(engine, args)
	case "logs":
		handleLogs(engine, args)
	case "rm":
		handleRm(engine, args)
	case "build":
		handleBuild(engine, args)
	case "version":
		handleVersion()
	case "dashboard":
		handleDashboard(engine)
	case "prune":
		handlePrune(engine)
	case "volume":
		handleVolume(engine, args)
	case "compose":
		handleCompose(engine, args)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("PocketLinx (plx) - Portable Container Runtime")
	fmt.Println("Usage:")
	fmt.Println("  plx setup                        Initialize environment")
	fmt.Println("  plx install                      Add plx to your system PATH")
	fmt.Println("  plx pull <image>                 Download an image (alpine, ubuntu)")
	fmt.Println("  plx images                       List downloaded images")
	fmt.Println("  plx run [-it] [-e K=V] [-p H:C] [-v S:D] [image] <cmd>...  Run command")
	fmt.Println("  plx ps                           List containers")
	fmt.Println("  plx stop <id>                    Stop container")
	fmt.Println("  plx logs <id>                    View container logs")
	fmt.Println("  plx rm <id>                      Remove container")
	fmt.Println("  plx build [path]                 Build image from Dockerfile")
	fmt.Println("  plx version                      Show version")
	fmt.Println("  plx dashboard                    Launch visual Control Center")
	fmt.Println("  plx prune                        Clear build cache")
	fmt.Println("  plx volume <create|ls|rm>        Manage persistent volumes")
	fmt.Println("  plx compose <up|down>            Orchestrate multiple containers (YAML-based)")
}

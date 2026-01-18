package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"PocketLinx/pkg/container"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var backend container.Backend

	// OS判定とバックエンドの選択
	switch runtime.GOOS {
	case "windows":
		backend = container.NewWSLBackend()
	case "linux":
		// 将来的に LinuxNativeBackend を実装予定
		fmt.Println("Native Linux backend is not yet implemented. Please use WSL2 for now.")
		os.Exit(1)
	default:
		fmt.Printf("OS %s is not supported yet.\n", runtime.GOOS)
		os.Exit(1)
	}

	engine := container.NewEngine(backend)

	switch os.Args[1] {
	case "setup":
		if err := engine.Setup(); err != nil {
			fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
			os.Exit(1)
		}
	case "pull":
		if len(os.Args) < 3 {
			fmt.Println("Usage: plx pull <image_name>")
			fmt.Println("Supported images: alpine, ubuntu")
			os.Exit(1)
		}
		if err := engine.Pull(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Pull failed: %v\n", err)
			os.Exit(1)
		}
	case "images":
		images, err := engine.Images()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list images: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%-20s\n", "IMAGE NAME")
		for _, img := range images {
			fmt.Printf("%-20s\n", img)
		}
	case "run":
		args := os.Args[2:]
		var mounts []container.Mount
		var cmdArgs []string
		image := "alpine"

		for i := 0; i < len(args); i++ {
			if args[i] == "-v" && i+1 < len(args) {
				val := args[i+1]
				lastColon := strings.LastIndex(val, ":")
				if lastColon != -1 && lastColon > 1 {
					mounts = append(mounts, container.Mount{
						Source: val[:lastColon],
						Target: val[lastColon+1:],
					})
				}
				i++
			} else if args[i] == "--image" && i+1 < len(args) {
				image = args[i+1]
				i++
			} else {
				cmdArgs = args[i:]
				break
			}
		}

		if len(cmdArgs) == 0 {
			fmt.Println("Usage: plx run [--image <name>] [-v host:container] <command> [args...]")
			os.Exit(1)
		}

		opts := container.RunOptions{
			Image:  image,
			Args:   cmdArgs,
			Mounts: mounts,
		}

		if err := engine.Run(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Run failed: %v\n", err)
			os.Exit(1)
		}
	case "ps":
		containers, err := engine.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list containers: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%-15s %-20s %-25s %-10s\n", "CONTAINER ID", "COMMAND", "CREATED", "STATUS")
		for _, c := range containers {
			fmt.Printf("%-15s %-20s %-25s %-10s\n",
				c.ID,
				c.Command,
				c.Created.Format("2006-01-02 15:04:05"),
				c.Status,
			)
		}
	case "rm":
		if len(os.Args) < 3 {
			fmt.Println("Usage: plx rm <container_id>")
			os.Exit(1)
		}
		if err := engine.Remove(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove container: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Container %s removed.\n", os.Args[2])

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("PocketLinx (plx) - Portable Container Runtime")
	fmt.Println("Usage:")
	fmt.Println("  plx setup                        Initialize environment")
	fmt.Println("  plx pull <image>                 Download an image (alpine, ubuntu)")
	fmt.Println("  plx images                       List downloaded images")
	fmt.Println("  plx run [--image <name>] [-v src:dst] <cmd>...  Run command")
	fmt.Println("  plx ps                           List containers")
	fmt.Println("  plx rm <id>                      Remove container")
}

package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
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
	case "install":
		if err := engine.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
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
		headers := []string{"IMAGE NAME"}
		var rows [][]string
		for _, img := range images {
			rows = append(rows, []string{img})
		}
		container.PrintTable(headers, rows)
	case "run":
		// 1. Load plx.json if exists (as defaults)
		config, _ := container.LoadProjectConfig()

		args := os.Args[2:]
		var mounts []container.Mount
		env := make(map[string]string)
		var cmdArgs []string
		var portMappings []container.PortMapping
		image := ""
		interactive := false

		// Apply config defaults
		if config != nil {
			image = config.Image
			mounts = append(mounts, config.Mounts...)
		}
		if image == "" {
			image = "alpine"
		}

		// 2. Parse command line flags (overrides config)
		imageSetByFlag := false
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if arg == "-v" && i+1 < len(args) {
				val := args[i+1]
				lastColon := strings.LastIndex(val, ":")
				if lastColon != -1 && lastColon > 1 {
					mounts = append(mounts, container.Mount{
						Source: val[:lastColon],
						Target: val[lastColon+1:],
					})
				}
				i++
			} else if arg == "-p" && i+1 < len(args) {
				val := args[i+1]
				parts := strings.Split(val, ":")
				if len(parts) == 2 {
					hPort, _ := strconv.Atoi(parts[0])
					cPort, _ := strconv.Atoi(parts[1])
					if hPort > 0 && cPort > 0 {
						portMappings = append(portMappings, container.PortMapping{
							Host:      hPort,
							Container: cPort,
						})
					}
				}
				i++
			} else if arg == "-e" && i+1 < len(args) {
				val := args[i+1]
				parts := strings.SplitN(val, "=", 2)
				if len(parts) == 2 {
					env[parts[0]] = parts[1]
				}
				i++
			} else if arg == "--image" && i+1 < len(args) {
				image = args[i+1]
				imageSetByFlag = true
				i++
			} else if arg == "-it" || arg == "-i" || arg == "-t" {
				interactive = true
			} else if strings.HasPrefix(arg, "-") {
				// Unknown flag
				fmt.Printf("Unknown flag: %s\n", arg)
			} else {
				// Non-flag argument: This is the image name if not set via flag, or the start of the command
				if !imageSetByFlag {
					image = arg
					cmdArgs = args[i+1:]
				} else {
					cmdArgs = args[i:]
				}
				break
			}
		}

		if len(cmdArgs) == 0 {
			fmt.Println("Usage: plx run [-it] [-e KEY=VAL] [-p HOST:CONT] [IMAGE] <command> [args...]")
			os.Exit(1)
		}

		opts := container.RunOptions{
			Image:       image,
			Args:        cmdArgs,
			Mounts:      mounts,
			Env:         env,
			Ports:       portMappings,
			Interactive: interactive,
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
		headers := []string{"CONTAINER ID", "COMMAND", "CREATED", "STATUS"}
		var rows [][]string
		for _, c := range containers {
			rows = append(rows, []string{
				c.ID,
				c.Command,
				c.Created.Format("2006-01-02 15:04:05"),
				c.Status,
			})
		}
		container.PrintTable(headers, rows)
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
	fmt.Println("  plx install                      Add plx to your system PATH")
	fmt.Println("  plx pull <image>                 Download an image (alpine, ubuntu)")
	fmt.Println("  plx images                       List downloaded images")
	fmt.Println("  plx run [-it] [--image <name>] [-v src:dst] <cmd>...  Run command")
	fmt.Println("  plx ps                           List containers")
	fmt.Println("  plx rm <id>                      Remove container")
}

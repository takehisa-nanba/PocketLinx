package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"PocketLinx/pkg/api"
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
		if os.Geteuid() != 0 {
			fmt.Println("Warning: PocketLinx on Linux requires root privileges (for unshare/mount). Please run with sudo.")
		}
		backend = container.NewLinuxBackend()
	default:
		fmt.Printf("OS %s is not supported yet.\n", runtime.GOOS)
		os.Exit(1)
	}

	engine := container.NewEngine(backend)

	switch os.Args[1] {
	case "setup":
		if err := container.CheckRequirements(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: System requirements not met:\n  %v\n", err)
			os.Exit(1)
		}
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
		handleRun(engine, os.Args[2:])
	case "ps":
		containers, err := engine.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list containers: %v\n", err)
			os.Exit(1)
		}
		headers := []string{"CONTAINER ID", "NAME", "COMMAND", "CREATED", "STATUS"}
		var rows [][]string
		for _, c := range containers {
			displayCmd := c.Command
			if len(displayCmd) > 30 {
				displayCmd = displayCmd[:27] + "..."
			}
			rows = append(rows, []string{
				c.ID,
				c.Name,
				displayCmd,
				c.Created.Format("2006-01-02 15:04:05"),
				c.Status,
			})
		}
		container.PrintTable(headers, rows)
	case "stop":
		if len(os.Args) < 3 {
			fmt.Println("Usage: plx stop <container_id>")
			os.Exit(1)
		}
		if err := engine.Stop(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop container: %v\n", err)
			os.Exit(1)
		}
	case "logs":
		if len(os.Args) < 3 {
			fmt.Println("Usage: plx logs <container_id>")
			os.Exit(1)
		}
		logs, err := engine.Logs(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get logs: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(logs)
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
	case "build":
		ctxDir := "."
		if len(os.Args) >= 3 {
			ctxDir = os.Args[2]
		}

		// Try to load config to get image name
		config, _ := container.LoadProjectConfigFromDir(ctxDir)
		targetImage := ""
		if config != nil && config.Image != "" {
			targetImage = config.Image
		}

		img, err := engine.Build(ctxDir, targetImage)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully built image: %s\n", img)
	case "version":
		fmt.Println("PocketLinx v0.3.0 (WSL Native Architecture)")
	case "dashboard":
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
	case "prune":
		fmt.Println("Pruning build cache...")
		if err := engine.Prune(); err != nil {
			fmt.Fprintf(os.Stderr, "Prune failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Build cache cleared.")
	case "volume":
		handleVolume(engine, os.Args[2:])
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
}

func handleVolume(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx volume <create|ls|rm> [args...]")
		os.Exit(1)
	}

	switch args[0] {
	case "create":
		if len(args) < 2 {
			fmt.Println("Usage: plx volume create <name>")
			os.Exit(1)
		}
		name := args[1]
		if err := engine.CreateVolume(name); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create volume: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Volume '%s' created.\n", name)
	case "rm":
		if len(args) < 2 {
			fmt.Println("Usage: plx volume rm <name>")
			os.Exit(1)
		}
		name := args[1]
		if err := engine.RemoveVolume(name); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove volume: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Volume '%s' removed.\n", name)
	case "ls":
		vols, err := engine.ListVolumes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list volumes: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("VOLUMES")
		for _, v := range vols {
			fmt.Println(v)
		}
	default:
		fmt.Printf("Unknown volume command: %s\n", args[0])
		os.Exit(1)
	}
}

func handleRun(engine *container.Engine, args []string) {
	opts, err := parseRunOptions(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing run options: %v\n", err)
		os.Exit(1)
	}

	if err := engine.Run(*opts); err != nil {
		fmt.Fprintf(os.Stderr, "Run failed: %v\n", err)
		os.Exit(1)
	}
}

func parseRunOptions(args []string) (*container.RunOptions, error) {
	// 1. Load plx.json if exists (as defaults)
	config, _ := container.LoadProjectConfig()

	var mounts []container.Mount
	env := make(map[string]string)
	var cmdArgs []string
	var portMappings []container.PortMapping
	image := ""
	interactive := false
	detach := false
	workdir := ""

	// Apply config defaults
	if config != nil {
		image = config.Image
		mounts = append(mounts, config.Mounts...)
		if len(config.Command) > 0 {
			cmdArgs = config.Command
		}
		if config.Workdir != "" {
			workdir = config.Workdir
		}
	}
	if image == "" {
		image = "alpine"
	}

	// 2. Parse command line flags (overrides config)
	imageSetByFlag := false
	name := "" // Parse --name

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
		} else if arg == "--name" && i+1 < len(args) { // Parse Name
			name = args[i+1]
			i++
		} else if arg == "-it" || arg == "-i" || arg == "-t" {
			interactive = true
		} else if arg == "-d" {
			detach = true
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
			// Explicit CLI args override config default
			break
		}
	}

	// Dockerfile Auto-detection if image/args are missing
	if image == "alpine" && len(cmdArgs) == 0 {
		if df, err := container.ParseDockerfile("Dockerfile"); err == nil {
			fmt.Println("Dockerfile detected. Using its configuration...")
			// Apply Dockerfile defaults
			absPath, _ := filepath.Abs(".")
			image = strings.ToLower(filepath.Base(absPath))

			// Reconstruct properties from Instructions
			for _, instr := range df.Instructions {
				switch instr.Type {
				case "CMD":
					// Simple shell/exec form detection
					args := instr.Raw
					if strings.HasPrefix(args, "[") && strings.HasSuffix(args, "]") {
						trimmed := strings.Trim(args, "[]")
						parts := strings.Split(trimmed, ",")
						for i, p := range parts {
							parts[i] = strings.Trim(strings.TrimSpace(p), "\"")
						}
						cmdArgs = parts
					} else {
						cmdArgs = []string{"sh", "-c", args}
					}
				case "ENV":
					for i := 0; i < len(instr.Args); i += 2 {
						k := instr.Args[i]
						v := ""
						if i+1 < len(instr.Args) {
							v = instr.Args[i+1]
						}
						if _, exists := env[k]; !exists {
							env[k] = v
						}
					}
				case "EXPOSE":
					for _, pStr := range instr.Args {
						if p, err := strconv.Atoi(pStr); err == nil {
							// Default mapping: host port same as container port
							portMappings = append(portMappings, container.PortMapping{Host: p, Container: p})
						}
					}
				case "WORKDIR":
					if len(instr.Args) > 0 {
						workdir = instr.Args[0]
					}
				}
			}
		}
	}

	if len(cmdArgs) == 0 {
		return nil, fmt.Errorf("Usage: plx run [-it] [-e KEY=VAL] [-p HOST:CONT] [IMAGE] <command> [args...]")
	}

	// Heuristic: If workdir is empty and we have a mount to /app, default to /app
	if workdir == "" {
		for _, m := range mounts {
			if m.Target == "/app" {
				workdir = "/app"
				break
			}
		}
	}

	return &container.RunOptions{
		Image:       image,
		Name:        name,
		Args:        cmdArgs,
		Mounts:      mounts,
		Env:         env,
		Ports:       portMappings,
		Interactive: interactive,
		Detach:      detach,
		Workdir:     workdir,
	}, nil
}

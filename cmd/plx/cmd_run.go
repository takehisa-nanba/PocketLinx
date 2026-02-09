package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"PocketLinx/pkg/container"
)

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

func handleUpdate(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx update <container_id> [options]")
		return
	}
	id := args[0]
	opts, err := parseRunOptions(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing options: %v\n", err)
		os.Exit(1)
	}

	if err := engine.Update(id, *opts); err != nil {
		fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Container %s updated.\n", id)
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
			source := ""
			target := ""

			// Windows absolute path: C:\path:/app
			// Look for ':' that isn't a drive letter colon (index 1)
			if len(val) >= 2 && val[1] == ':' {
				// Found drive letter, search for the NEXT colon
				colonIdx := strings.Index(val[2:], ":")
				if colonIdx != -1 {
					source = val[:colonIdx+2]
					target = val[colonIdx+3:]
				}
			} else {
				// Linux path or relative: /path:/app or ./path:/app
				colonIdx := strings.Index(val, ":")
				if colonIdx != -1 {
					source = val[:colonIdx]
					target = val[colonIdx+1:]
				}
			}

			if source != "" && target != "" {
				// Guard against malformed colons or empty paths (v0.8.1)
				if strings.Contains(target, ":") || source == "" || target == "" {
					fmt.Printf("Warning: Invalid mount format '%s'. Skipping.\n", val)
					continue
				}
				mounts = append(mounts, container.Mount{
					Source: source,
					Target: target,
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
		} else if (arg == "--name" || arg == "-n") && i+1 < len(args) { // Parse Name
			name = args[i+1]
			i++
		} else if arg == "-it" || arg == "-i" || arg == "-t" {
			interactive = true
		} else if arg == "-d" || arg == "--detach" {
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
			// Sanitize image name (v0.8.1: replace spaces and special chars with hyphens)
			image = strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					return r
				}
				return '-'
			}, image)

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

	if len(cmdArgs) == 0 && image == "alpine" {
		return nil, fmt.Errorf("Usage: plx run [options] <image> [command] [args...]\nOptions: -it, -d, -v, -p, -e, --name")
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

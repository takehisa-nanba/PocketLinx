package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"PocketLinx/pkg/compose"
	"PocketLinx/pkg/container"
)

func handleCompose(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx compose <up|down>")
		os.Exit(1)
	}

	command := args[0]
	// Detect compose file
	// TODO: Support -f flag
	composeFile := "plx-compose.yml"
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: %s not found\n", composeFile)
		os.Exit(1)
	}

	// Parse
	config, err := compose.ParseComposeFile(composeFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing compose file: %v\n", err)
		os.Exit(1)
	}

	projectName := "default"
	if abs, err := filepath.Abs("."); err == nil {
		projectName = strings.ToLower(filepath.Base(abs))
	}

	switch command {
	case "up":
		runComposeUp(engine, config, projectName)
	case "down":
		runComposeDown(engine, config, projectName)
	default:
		fmt.Printf("Unknown compose command: %s\n", command)
		os.Exit(1)
	}
}

func runComposeUp(engine *container.Engine, config *compose.ComposeConfig, projectName string) {
	fmt.Printf("Starting project '%s'...\n", projectName)

	runningServices := make(map[string]string) // name -> ip

	// Naive sequential start (Order is random due to map, ideally topo sort)
	// For now, map iteration order.
	for name, svc := range config.Services {
		fmt.Printf("Creating %s_%s ...\n", projectName, name)

		containerName := fmt.Sprintf("%s_%s", projectName, name)

		// Map ServiceConfig to RunOptions
		opts := container.RunOptions{
			Image:       svc.Image,
			Name:        containerName,
			Args:        svc.Command,
			Detach:      true, // Generally compose up matches detach or attaches all. Let's default to detach for now.
			Interactive: false,
		}

		// Service Discovery: Add previously started services to ExtraHosts
		for sName, sIP := range runningServices {
			opts.ExtraHosts = append(opts.ExtraHosts, fmt.Sprintf("%s:%s", sName, sIP))
		}

		// Environment
		env := make(map[string]string)
		for _, e := range svc.Environment {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}
		opts.Env = env

		// Ports
		for _, p := range svc.Ports {
			parts := strings.Split(p, ":")
			if len(parts) == 2 {
				h, _ := strconv.Atoi(parts[0])
				c, _ := strconv.Atoi(parts[1])
				opts.Ports = append(opts.Ports, container.PortMapping{Host: h, Container: c})
			}
		}

		// Volumes
		for _, v := range svc.Volumes {
			parts := strings.Split(v, ":")
			if len(parts) >= 2 {
				opts.Mounts = append(opts.Mounts, container.Mount{Source: parts[0], Target: parts[1]})
			}
		}

		// Run
		if err := engine.Run(opts); err != nil {
			fmt.Printf("Failed to start %s: %v\n", name, err)
		} else {
			fmt.Printf("Started %s\n", name)
			// Track it for discovery of next services
			// Since they share network, it's 127.0.0.1, but using the service name as hostname
			runningServices[name] = "127.0.0.1"
		}
	}
}

func runComposeDown(engine *container.Engine, config *compose.ComposeConfig, projectName string) {
	fmt.Printf("Stopping project '%s'...\n", projectName)

	// List all containers, find matches (prefix based)
	// Better: Use labels, but we only have Name for now.
	// We can iterate defined services and guess names.

	for name := range config.Services {
		containerName := fmt.Sprintf("%s_%s", projectName, name)

		// Try stop
		fmt.Printf("Stopping %s ...\n", containerName)
		// Need actual ID lookup?
		// Engine.Remove/Stop usually takes ID, but let's see if we can support Names later.
		// Current Engine.List() returns list. We scan it.

		containers, _ := engine.List()
		for _, c := range containers {
			if c.Name == containerName {
				_ = engine.Stop(c.ID)
				_ = engine.Remove(c.ID)
				fmt.Printf("Removed %s\n", containerName)
			}
		}
	}
}

package main

import (
	"fmt"
	"os"

	"PocketLinx/pkg/container"
)

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

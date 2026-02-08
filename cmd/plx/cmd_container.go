package main

import (
	"fmt"
	"os"

	"PocketLinx/pkg/container"
)

func handlePs(engine *container.Engine) {
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
}

func handleStop(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx stop <container_id>")
		os.Exit(1)
	}
	if err := engine.Stop(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stop container: %v\n", err)
		os.Exit(1)
	}
}

func handleStart(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx start <container_id>")
		os.Exit(1)
	}
	if err := engine.Start(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start container: %v\n", err)
		os.Exit(1)
	}
}

func handleLogs(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx logs <container_id>")
		os.Exit(1)
	}
	logs, err := engine.Logs(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get logs: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(logs)
}

func handleRm(engine *container.Engine, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plx rm <container_id> [--all]")
		os.Exit(1)
	}

	if args[0] == "--all" {
		containers, err := engine.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list containers: %v\n", err)
			os.Exit(1)
		}
		for _, c := range containers {
			if err := engine.Remove(c.ID); err != nil {
				fmt.Printf("Warning: Failed to remove %s: %v\n", c.ID, err)
			}
		}
		fmt.Println("Container --all removed.")
		return
	}

	if err := engine.Remove(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove container: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Container %s removed.\n", args[0])
}

func handleExec(engine *container.Engine, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: plx exec [-it] <container> <cmd>...")
		os.Exit(1)
	}

	interactive := false
	containerName := ""
	var cmdArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-it" || arg == "-i" || arg == "-t" {
			interactive = true
		} else if containerName == "" {
			containerName = arg
		} else {
			cmdArgs = args[i:]
			break
		}
	}

	if containerName == "" || len(cmdArgs) == 0 {
		fmt.Println("Usage: plx exec [-it] <container> <cmd>...")
		os.Exit(1)
	}

	if err := engine.Exec(containerName, cmdArgs, interactive); err != nil {
		fmt.Fprintf(os.Stderr, "Exec failed: %v\n", err)
		os.Exit(1)
	}
}

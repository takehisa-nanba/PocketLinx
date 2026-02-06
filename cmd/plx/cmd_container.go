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
		fmt.Println("Usage: plx rm <container_id>")
		os.Exit(1)
	}
	if err := engine.Remove(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove container: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Container %s removed.\n", args[0])
}

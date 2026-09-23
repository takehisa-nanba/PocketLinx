package main

import (
	"PocketLinx/pkg/wslbridge"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

func runWSL(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("wsl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	distro := fs.String("distro", "", "explicit independent WSL2 distribution")
	cli := fs.String("linux-cli", "/usr/local/bin/plx-env", "trusted Linux CLI absolute path")
	user := fs.String("user", "root", "explicit Linux user (experimental run requires root)")
	stopped := fs.Bool("stopped", false, "assert environment writers stopped (save only)")
	trusted := fs.Bool("trusted-sample", false, "acknowledge run is only for the trusted dedicated sample")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("wsl --distro NAME [flags] check | restore WINDOWS_BUNDLE LINUX_DIR | run LINUX_DIR | save LINUX_DIR WINDOWS_BUNDLE")
	}
	a := wslbridge.Adapter{Distro: *distro, LinuxCLI: *cli, User: *user, Invoke: wslbridge.SystemRunner}
	ctx := context.Background()
	switch rest[0] {
	case "check":
		if len(rest) == 1 {
			return a.Check(ctx, os.Stderr)
		}
	case "restore":
		if len(rest) == 3 {
			return a.Restore(ctx, rest[1], rest[2], os.Stderr)
		}
	case "run":
		if len(rest) == 2 {
			if !*trusted {
				return fmt.Errorf("run requires --trusted-sample; arbitrary received environments are not supported")
			}
			return a.Run(ctx, rest[1], os.Stdin, out, os.Stderr)
		}
	case "save":
		if len(rest) == 3 {
			if !*stopped {
				return fmt.Errorf("save requires --stopped")
			}
			return a.Save(ctx, rest[1], rest[2], os.Stderr)
		}
	}
	return fmt.Errorf("invalid wsl operation or arguments; flags must precede operation")
}

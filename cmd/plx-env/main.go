// plx-env is the experimental, runtime-independent environment bundle CLI.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"PocketLinx/pkg/environment"
)

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: plx-env save --stopped SOURCE BUNDLE | plx-env restore BUNDLE DESTINATION")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(out)
	maxBytes := fs.Int64("max-bytes", 8<<30, "maximum total payload bytes")
	maxEntries := fs.Int("max-entries", 100000, "maximum file/directory/link count")
	stopped := fs.Bool("stopped", false, "assert all source processes and external writers are stopped (save only)")
	if args[0] != "save" && args[0] != "restore" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("%s requires exactly two paths; flags precede paths", args[0])
	}
	if *maxBytes <= 0 || *maxEntries <= 0 {
		return fmt.Errorf("limits must be positive")
	}
	limits := environment.Limits{MaxBytes: *maxBytes, MaxEntries: *maxEntries}
	var err error
	if args[0] == "save" {
		err = environment.Save(fs.Arg(0), fs.Arg(1), environment.SaveOptions{Stopped: *stopped, Limits: limits})
	} else {
		if *stopped {
			return fmt.Errorf("--stopped applies only to save")
		}
		err = environment.Restore(fs.Arg(0), fs.Arg(1), limits)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Completed:", fs.Arg(1))
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

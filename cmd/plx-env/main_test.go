package main

import (
	"io"
	"testing"
)

func TestUsageFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"exec"}, {"save"}, {"restore", "--stopped", "a", "b"}, {"restore", "--max-bytes=-1", "a", "b"}, {"save", "a", "b", "c"}} {
		if err := run(args, io.Discard); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

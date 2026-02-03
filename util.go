package main

import (
	"context"
	"os/exec"
)

func command(ctx context.Context, dir, cmdline string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, cmdline, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

func execute(ctx context.Context, dir, cmdline string, args ...string) ([]byte, error) {
	return command(ctx, dir, cmdline, args...).CombinedOutput()
}

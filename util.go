package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

func command(ctx context.Context, dir, cmdline string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, cmdline, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = nil
	return cmd
}

func execute(ctx context.Context, dir, cmdline string, args ...string) ([]byte, error) {
	output, err := command(ctx, dir, cmdline, args...).CombinedOutput()
	return output, err
}

func cleanOutputJSON(b []byte) ([]byte, error) {
	output := bytes.TrimRight(b, " \t\n\r")
	idx := bytes.IndexByte(output, '{')
	if idx == -1 {
		return nil, fmt.Errorf("no JSON object found in data: %q", output)
	}
	return output[idx:], nil
}

func executeJSON(ctx context.Context, i any, dir, cmdline string, args ...string) error {
	output, err := execute(ctx, dir, cmdline, args...)
	if err != nil {
		return err
	}
	output, err = cleanOutputJSON(output)
	if err != nil {
		return err
	}
	return json.Unmarshal(output, i)
}

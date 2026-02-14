package main

import "context"

const defaultOcModel = "github-copilot/claude-sonnet-4.5"

func ocCommand(ctx context.Context, path, model, cmd string, args ...string) ([]byte, error) {
	cmdArgs := []string{"run", "-m", model, "--command", cmd}
	cmdArgs = append(cmdArgs, args...)
	return execute(ctx, path, "opencode", cmdArgs...)
}

func ocPrompt(ctx context.Context, path, model, prompt string) ([]byte, error) {
	if model == "" {
		model = defaultOcModel
	}
	cmdArgs := []string{"run", "-m", model, prompt}
	return execute(ctx, path, "opencode", cmdArgs...)
}

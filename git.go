package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func createBranch(ctx context.Context, repoPath, owner, repo, branch string) error {
	repoLink := fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
	if _, err := execute(ctx, repoPath, "git", "clone", "--branch", branch, "--single-branch", repoLink, branch); err != nil {
		return err
	}
	return nil
}

func pull(ctx context.Context, path string) error {
	_, err := execute(ctx, path, "git", "pull")
	return err
}

func clone(ctx context.Context, path, owner, repo, branch string) error {
	projectPath := filepath.Join(path, repo)
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}
	return createBranch(ctx, projectPath, owner, repo, branch)
}

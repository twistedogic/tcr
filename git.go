package main

import (
	"context"
	"fmt"
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

func clone(ctx context.Context, path, owner, repo string) error {
	repoLink := fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
	_, err := execute(ctx, path, "git", "clone", repoLink)
	return err
}

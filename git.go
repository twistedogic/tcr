package main

import (
	"context"
	"fmt"
)

func createWorktree(ctx context.Context, repo, tree string) error {
	if _, err := execute(ctx, repo, "git", "worktree", "add", tree); err != nil {
		return err
	}
	if _, err := execute(ctx, tree, "openspec", "init", "--tools", "opencode", "--force"); err != nil {
		return err
	}
	return nil
}

func deleteWorktree(ctx context.Context, repo, tree string) error {
	_, err := execute(ctx, repo, "git", "worktree", "remove", tree, "--force")
	return err
}

func commit(ctx context.Context, path, message string) error {
	if _, err := execute(ctx, path, "git", "add", "."); err != nil {
		return err
	}
	_, err := execute(ctx, path, "git", "commit", "-m", message)
	return err
}

func amendCommit(ctx context.Context, path string) error {
	if _, err := execute(ctx, path, "git", "add", "."); err != nil {
		return err
	}
	_, err := execute(ctx, path, "git", "commit", "--amend", "--no-edit")
	return err
}

func push(ctx context.Context, path string) error {
	_, err := execute(ctx, path, "git", "push", "-f")
	return err
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

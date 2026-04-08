package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checkoutBranch checks out a branch in an existing local git repo.
// If the branch exists on the remote, it tracks it. Otherwise it creates a new local branch.
func checkoutBranch(ctx context.Context, repoPath, branch string) error {
	// Fetch latest from remote (best-effort — ignore errors for offline use)
	_, _ = execute(ctx, repoPath, "git", "fetch", "--all")

	// Try to checkout existing local or remote-tracking branch
	if _, err := execute(ctx, repoPath, "git", "checkout", branch); err == nil {
		return nil
	}

	// Try to checkout as remote tracking branch (origin/<branch>)
	if _, err := execute(ctx, repoPath, "git", "checkout", "-b", branch, fmt.Sprintf("origin/%s", branch)); err == nil {
		return nil
	}

	// Create a brand new local branch
	if _, err := execute(ctx, repoPath, "git", "checkout", "-b", branch); err != nil {
		return fmt.Errorf("checkout branch %q: %w", branch, err)
	}
	return nil
}

// listBranches returns all local branch names in a repo.
func listBranches(ctx context.Context, repoPath string) ([]string, error) {
	out, err := execute(ctx, repoPath, "git", "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// currentBranch returns the currently checked-out branch name.
func currentBranch(ctx context.Context, repoPath string) (string, error) {
	out, err := execute(ctx, repoPath, "git", "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func pull(ctx context.Context, path string) error {
	_, err := execute(ctx, path, "git", "pull")
	return err
}

// clone clones a GitHub repo into workspace/<repo>/ checking out the given branch.
func clone(ctx context.Context, workspace, owner, repo, branch string) error {
	repoLink := fmt.Sprintf("git@github.com:%s/%s.git", owner, repo)
	projectPath := filepath.Join(workspace, repo)
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}
	tmpPath := projectPath + ".tmp"
	_ = os.RemoveAll(tmpPath)
	if _, err := execute(ctx, workspace, "git", "clone", "--branch", branch, repoLink, tmpPath); err != nil {
		_ = os.RemoveAll(tmpPath)
		return fmt.Errorf("clone %s/%s: %w", owner, repo, err)
	}
	if err := os.Rename(tmpPath, projectPath); err != nil {
		_ = os.RemoveAll(tmpPath)
		return err
	}
	return nil
}

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseOrigin(t *testing.T) {
	cases := map[string]struct {
		origin string
		owner  string
		repo   string
	}{
		"ssh url": {
			origin: "git@github.com:owner/repo.git",
			owner:  "owner",
			repo:   "repo",
		},
		"https url": {
			origin: "https://github.com/owner/repo.git",
			owner:  "owner",
			repo:   "repo",
		},
		"ssh url without .git": {
			origin: "git@github.com:owner/repo",
			owner:  "owner",
			repo:   "repo",
		},
		"https url without .git": {
			origin: "https://github.com/owner/repo",
			owner:  "owner",
			repo:   "repo",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			owner, repo, err := parseOrigin(tc.origin)
			require.NoError(t, err)
			require.Equal(t, tc.owner, owner)
			require.Equal(t, tc.repo, repo)
		})
	}
}

func Test_parseOrigin_invalid(t *testing.T) {
	cases := []struct {
		name   string
		origin string
	}{
		{"empty", ""},
		{"no owner", "repo"},
		{"unsupported protocol", "ssh://github.com/owner/repo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseOrigin(tc.origin)
			require.Error(t, err)
		})
	}
}

func Test_compareWorktree(t *testing.T) {
	wt1 := &Worktree{Name: "a"}
	wt2 := &Worktree{Name: "b"}

	require.Equal(t, -1, compareWorktree(wt1, wt2))
	require.Equal(t, 1, compareWorktree(wt2, wt1))
	require.Equal(t, 0, compareWorktree(wt1, wt1))
}

func TestProject_Title(t *testing.T) {
	p := &Project{owner: "myowner", repo: "myrepo"}
	require.Equal(t, "myowner/myrepo", p.Title())
}

func TestProject_Description(t *testing.T) {
	p := &Project{
		owner:     "myowner",
		repo:      "myrepo",
		worktrees: []*Worktree{{}, {}, {}},
	}
	require.Equal(t, "", p.Description())
}

func TestProject_FilterValue(t *testing.T) {
	p := &Project{owner: "myowner", repo: "myrepo"}
	require.Equal(t, "myowner/myrepo", p.FilterValue())
}

func TestWorktree_Title(t *testing.T) {
	wt := &Worktree{Owner: "owner", Repo: "repo", Name: "feature"}
	require.Equal(t, "owner/repo – feature", wt.Title())
}

func TestWorktree_Description(t *testing.T) {
	wt := &Worktree{Name: "feature"}
	require.Equal(t, "", wt.Description())
}

func TestWorktree_FilterValue(t *testing.T) {
	wt := &Worktree{Name: "feature-branch"}
	require.Equal(t, "feature-branch", wt.FilterValue())
}

func setupBareRepo(t *testing.T) (remoteDir, localDir string) {
	t.Helper()
	remote := t.TempDir()
	local := t.TempDir()

	_, err := exec.Command("git", "-C", remote, "init", "--bare").CombinedOutput()
	require.NoError(t, err)

	_, err = exec.Command("git", "clone", remote, local).CombinedOutput()
	require.NoError(t, err)

	_, err = exec.Command("git", "-C", local, "config", "user.email", "test@test.com").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "config", "user.name", "Test").CombinedOutput()
	require.NoError(t, err)

	f := filepath.Join(local, "README.md")
	require.NoError(t, os.WriteFile(f, []byte("hello"), 0644))
	_, err = exec.Command("git", "-C", local, "add", ".").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "push", "-u", "origin", "HEAD").CombinedOutput()
	require.NoError(t, err)

	return remote, local
}

func TestCheckoutBranch_newBranch(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	err := checkoutBranch(ctx, local, "feature-x")
	require.NoError(t, err)

	out, err := exec.Command("git", "-C", local, "branch", "--show-current").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "feature-x", strings.TrimSpace(string(out)))
}

func TestCheckoutBranch_existingRemoteBranch(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	_, err := exec.Command("git", "-C", local, "checkout", "-b", "existing-branch").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "push", "-u", "origin", "existing-branch").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "checkout", "main").CombinedOutput()
	require.NoError(t, err)

	err = checkoutBranch(ctx, local, "existing-branch")
	require.NoError(t, err)

	out, err := exec.Command("git", "-C", local, "branch", "--show-current").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "existing-branch", strings.TrimSpace(string(out)))
}

func TestProject_Refresh_readsGitBranches(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	_, err := exec.Command("git", "-C", local, "checkout", "-b", "feature-a").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "checkout", "main").CombinedOutput()
	require.NoError(t, err)

	p := &Project{owner: "o", repo: "r", path: local}
	require.NoError(t, p.Refresh(ctx))
	require.Len(t, p.worktrees, 2)

	names := make([]string, len(p.worktrees))
	for i, wt := range p.worktrees {
		names[i] = wt.Name
		require.Equal(t, local, wt.Path)
	}
	require.ElementsMatch(t, []string{"main", "feature-a"}, names)
}

func TestProject_AddWorktree_checksOutBranch(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	p := &Project{owner: "o", repo: "r", path: local}
	require.NoError(t, p.Refresh(ctx))
	initialCount := len(p.worktrees)

	require.NoError(t, p.AddWorktree(ctx, "new-feature"))
	require.Len(t, p.worktrees, initialCount+1)

	out, err := exec.Command("git", "-C", local, "branch", "--show-current").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "new-feature", strings.TrimSpace(string(out)))
}

func TestProject_DeleteWorktree_deletesBranch(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	_, err := exec.Command("git", "-C", local, "checkout", "-b", "to-delete").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "checkout", "main").CombinedOutput()
	require.NoError(t, err)

	p := &Project{owner: "o", repo: "r", path: local}
	require.NoError(t, p.Refresh(ctx))

	require.NoError(t, p.DeleteWorktree(ctx, "to-delete"))

	branches, err := listBranches(ctx, local)
	require.NoError(t, err)
	require.NotContains(t, branches, "to-delete")
	for _, wt := range p.worktrees {
		require.NotEqual(t, "to-delete", wt.Name)
	}
}

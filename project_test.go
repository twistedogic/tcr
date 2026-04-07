package main

import (
	"os"
	"path/filepath"
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
	require.Equal(t, "3 branches", p.Description())
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
	wt := &Worktree{Status: &Status{ChangeName: "my-change", ApplyRequires: []string{"tasks"}}}
	require.Equal(t, "Pending – tasks", wt.Description())
}

func TestWorktree_DescriptionNil(t *testing.T) {
	wt := &Worktree{}
	require.Equal(t, "No openspec setup", wt.Description())
}

func TestWorktree_FilterValue(t *testing.T) {
	wt := &Worktree{Name: "feature-branch"}
	require.Equal(t, "feature-branch", wt.FilterValue())
}

func TestProject_DeleteWorktree(t *testing.T) {
	tempDir := t.TempDir()

	// Create a project with a worktree directory
	worktreeName := "feature"
	worktreePath := filepath.Join(tempDir, worktreeName)
	require.NoError(t, os.MkdirAll(worktreePath, 0755))

	p := &Project{
		owner:     "test",
		repo:      "test",
		path:      tempDir,
		worktrees: []*Worktree{{Name: worktreeName}},
	}

	// Verify the directory exists
	_, err := os.Stat(worktreePath)
	require.NoError(t, err)

	// Delete the worktree
	err = p.DeleteWorktree(nil, worktreeName)
	require.NoError(t, err)

	// Verify the directory is gone
	_, err = os.Stat(worktreePath)
	require.True(t, os.IsNotExist(err))

	// Verify the worktree was removed from the list
	require.Len(t, p.worktrees, 0)
}

func TestProject_DeleteWorktree_notFound(t *testing.T) {
	p := &Project{
		owner:     "test",
		repo:      "test",
		path:      t.TempDir(),
		worktrees: []*Worktree{{Name: "nonexistent"}},
	}

	// Should not error even if directory doesn't exist
	err := p.DeleteWorktree(nil, "nonexistent")
	require.NoError(t, err)
}

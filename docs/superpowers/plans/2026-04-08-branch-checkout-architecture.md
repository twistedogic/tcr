# Branch Checkout Architecture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the per-branch-clone model with a single repo checkout per project, where branches are managed via `git fetch`/`git checkout`/`git branch`.

**Architecture:** Each project lives at `workspace/<repo>/` as a single git clone. `Worktree` represents a branch name (not a directory). `Project.Refresh` reads branches from `git branch -a` output. Creating a branch = `git fetch && git checkout -b <name>` or `git checkout <name>` for existing remote branches.

**Tech Stack:** Go, `github.com/charmbracelet/huh`, `github.com/charmbracelet/bubbletea`, `github.com/stretchr/testify`

---

### Task 1: Update `git.go` — replace `createBranch` with `checkoutBranch`

**Files:**
- Modify: `git.go`

- [ ] **Step 1: Write the failing test**

Add to `project_test.go` (these are integration tests requiring a real git repo — use `t.TempDir()` + `git init`):

```go
func setupBareRepo(t *testing.T) (remoteDir, localDir string) {
	t.Helper()
	remote := t.TempDir()
	local := t.TempDir()

	// Init bare remote
	_, err := exec.Command("git", "-C", remote, "init", "--bare").CombinedOutput()
	require.NoError(t, err)

	// Clone it locally
	_, err = exec.Command("git", "clone", remote, local).CombinedOutput()
	require.NoError(t, err)

	// Set up identity for commits
	_, err = exec.Command("git", "-C", local, "config", "user.email", "test@test.com").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "config", "user.name", "Test").CombinedOutput()
	require.NoError(t, err)

	// Create an initial commit so HEAD exists
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

	// Verify we're on feature-x
	out, err := exec.Command("git", "-C", local, "branch", "--show-current").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "feature-x", strings.TrimSpace(string(out)))
}

func TestCheckoutBranch_existingRemoteBranch(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	// Create a branch in another clone and push it
	local2 := t.TempDir()
	remote := filepath.Join(local, ".git", "config") // grab remote URL
	_ = remote
	// Push a new branch from local
	_, err := exec.Command("git", "-C", local, "checkout", "-b", "existing-branch").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "push", "-u", "origin", "existing-branch").CombinedOutput()
	require.NoError(t, err)
	// Back to main
	_, err = exec.Command("git", "-C", local, "checkout", "main").CombinedOutput()
	require.NoError(t, err)
	_ = local2

	err = checkoutBranch(ctx, local, "existing-branch")
	require.NoError(t, err)

	out, err := exec.Command("git", "-C", local, "branch", "--show-current").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "existing-branch", strings.TrimSpace(string(out)))
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./... -run TestCheckoutBranch -v
```
Expected: compile error — `checkoutBranch` undefined

- [ ] **Step 3: Implement `checkoutBranch` in `git.go`**

Replace the entire `git.go` with:

```go
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

	// Try to checkout existing remote branch
	if out, err := execute(ctx, repoPath, "git", "checkout", branch); err == nil {
		_ = out
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
	// Clone into a temp name then rename to avoid partial clones polluting the workspace
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run TestCheckoutBranch -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add git.go project_test.go
git commit -m "feat: replace per-branch clone with checkoutBranch"
```

---

### Task 2: Update `project.go` — single-clone project model

**Files:**
- Modify: `project.go`

The `Project` struct no longer has per-branch subdirectories. `project.path` is the repo root. `Worktree.Path` is always `project.path`. `Refresh` reads branches via `listBranches`. `AddWorktree` calls `checkoutBranch`. `DeleteWorktree` runs `git branch -d`. `LoadProject` reads origin from `project.path` directly.

- [ ] **Step 1: Write failing tests**

Add to `project_test.go`:

```go
func TestProject_Refresh_readsGitBranches(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	// Create a second branch
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

	// Verify git is on new-feature
	out, err := exec.Command("git", "-C", local, "branch", "--show-current").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "new-feature", strings.TrimSpace(string(out)))
}

func TestProject_DeleteWorktree_deletesBranch(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	// Create a branch to delete
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
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./... -run "TestProject_Refresh_readsGitBranches|TestProject_AddWorktree_checksOutBranch|TestProject_DeleteWorktree_deletesBranch" -v
```
Expected: FAIL — existing logic reads directories, not git branches

- [ ] **Step 3: Rewrite `project.go`**

Replace the entire file:

```go
package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Worktree struct {
	Owner string
	Repo  string
	Name  string
	Path  string
}

func (w *Worktree) refresh() error { return nil }

func (w *Worktree) Title() string       { return fmt.Sprintf("%s/%s – %s", w.Owner, w.Repo, w.Name) }
func (w *Worktree) Description() string { return "" }
func (w *Worktree) FilterValue() string { return w.Name }

func compareWorktree(a, b *Worktree) int { return cmp.Compare(a.Name, b.Name) }

type Project struct {
	repo  string
	owner string
	path  string

	worktrees []*Worktree
}

func (p *Project) Title() string       { return fmt.Sprintf("%s/%s", p.owner, p.repo) }
func (p *Project) Description() string { return fmt.Sprintf("%d branches", len(p.worktrees)) }
func (p *Project) FilterValue() string { return p.Title() }

func (p *Project) Refresh(ctx context.Context) error {
	branches, err := listBranches(ctx, p.path)
	if err != nil {
		if os.IsNotExist(err) {
			p.worktrees = nil
			return nil
		}
		return err
	}

	wts := make([]*Worktree, 0, len(branches))
	for _, b := range branches {
		wts = append(wts, &Worktree{
			Name:  b,
			Path:  p.path,
			Owner: p.owner,
			Repo:  p.repo,
		})
	}
	slices.SortFunc(wts, compareWorktree)
	p.worktrees = wts
	return nil
}

func (p *Project) AddWorktree(ctx context.Context, name string) error {
	if err := checkoutBranch(ctx, p.path, name); err != nil {
		return err
	}
	wt := &Worktree{Name: name, Path: p.path, Owner: p.owner, Repo: p.repo}
	if idx, exist := slices.BinarySearchFunc(p.worktrees, wt, compareWorktree); exist {
		p.worktrees[idx] = wt
	} else {
		p.worktrees = append(p.worktrees, wt)
		slices.SortFunc(p.worktrees, compareWorktree)
	}
	return nil
}

func (p *Project) DeleteWorktree(ctx context.Context, name string) error {
	if _, err := execute(ctx, p.path, "git", "branch", "-d", name); err != nil {
		// Force-delete if not fully merged
		if _, err2 := execute(ctx, p.path, "git", "branch", "-D", name); err2 != nil {
			return fmt.Errorf("delete branch %q: %w", name, err)
		}
	}
	idx, found := slices.BinarySearchFunc(p.worktrees, &Worktree{Name: name}, compareWorktree)
	if found {
		p.worktrees = append(p.worktrees[:idx], p.worktrees[idx+1:]...)
	}
	return nil
}

func parseOrigin(origin string) (owner, repo string, err error) {
	var title string
	if r, ok := strings.CutPrefix(origin, "https://github.com/"); ok {
		title = r
	} else if r, ok := strings.CutPrefix(origin, "git@github.com:"); ok {
		title = r
	} else {
		err = fmt.Errorf("unsupported remote origin: %s", origin)
		return
	}
	title = strings.TrimSuffix(title, ".git")
	var found bool
	owner, repo, found = strings.Cut(title, "/")
	if !found || owner == "" || repo == "" {
		err = fmt.Errorf("could not parse owner/repo from origin: %s", origin)
	}
	return
}

// LoadProject loads a project from a single git clone at path.
func LoadProject(ctx context.Context, path string) (*Project, error) {
	b, err := execute(ctx, path, "git", "remote", "get-url", "origin")
	if err != nil {
		return nil, fmt.Errorf("could not determine repo origin from %s: %w", path, err)
	}
	owner, repo, err := parseOrigin(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, err
	}
	p := &Project{owner: owner, repo: repo, path: path}
	return p, p.Refresh(ctx)
}

const maxConcurrency = 4

func LoadProjects(ctx context.Context, workspace string) ([]*Project, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, err
	}

	type result struct {
		idx     int
		project *Project
		err     error
	}

	ch := make(chan result, len(entries))
	sem := make(chan struct{}, maxConcurrency)

	idx := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		i := idx
		dirName := entry.Name()
		idx++
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			p, err := LoadProject(ctx, filepath.Join(workspace, dirName))
			ch <- result{idx: i, project: p, err: err}
		}()
	}

	projects := make([]*Project, 0, idx)
	for range idx {
		r := <-ch
		if r.err == nil && r.project != nil {
			projects = append(projects, r.project)
		}
	}
	return projects, nil
}

// LoadWorkspace is an alias for LoadProjects for backward compatibility.
// Deprecated: Use LoadProjects directly.
func LoadWorkspace(ctx context.Context, workspace string) ([]*Project, error) {
	return LoadProjects(ctx, workspace)
}
```

- [ ] **Step 4: Run all tests**

```bash
go test ./... -v
```
Expected: all pass. Note: `TestProject_DeleteWorktree` and `TestProject_DeleteWorktree_notFound` test the old directory-removal behavior — update them in the next step.

- [ ] **Step 5: Update stale tests in `project_test.go`**

`TestProject_DeleteWorktree` and `TestProject_DeleteWorktree_notFound` tested `os.RemoveAll`. Replace them:

```go
func TestProject_DeleteWorktree(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	// Create a branch to delete (must not be current branch)
	_, err := exec.Command("git", "-C", local, "checkout", "-b", "to-delete").CombinedOutput()
	require.NoError(t, err)
	_, err = exec.Command("git", "-C", local, "checkout", "main").CombinedOutput()
	require.NoError(t, err)

	p := &Project{owner: "o", repo: "r", path: local, worktrees: []*Worktree{{Name: "to-delete"}, {Name: "main"}}}
	require.NoError(t, p.DeleteWorktree(ctx, "to-delete"))

	require.Len(t, p.worktrees, 1)
	require.Equal(t, "main", p.worktrees[0].Name)

	branches, err := listBranches(ctx, local)
	require.NoError(t, err)
	require.NotContains(t, branches, "to-delete")
}

func TestProject_DeleteWorktree_notFound(t *testing.T) {
	_, local := setupBareRepo(t)
	ctx := context.Background()

	p := &Project{owner: "o", repo: "r", path: local, worktrees: []*Worktree{{Name: "nonexistent"}}}
	err := p.DeleteWorktree(ctx, "nonexistent")
	require.Error(t, err)
}
```

- [ ] **Step 6: Run all tests**

```bash
go test ./... -v
```
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add project.go project_test.go
git commit -m "feat: single-clone project model with git branch management"
```

---

### Task 3: Update `app.go` — form labels and `Worktree.Path` usage

**Files:**
- Modify: `app.go`

The `newWorktreeForm` title and field label need updating. Also, `interactive` calls use `m.selectedWorktree.Path` — this now always equals the repo root, so no logic change is needed, but verify it still compiles and works.

- [ ] **Step 1: Update `newWorktreeForm` in `app.go`**

Find:

```go
func newWorktreeForm(repoName string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Key("name").Title("worktree name").Validate(huh.ValidateNotEmpty()),
		).Title(fmt.Sprintf("%s – add new worktree", repoName)),
	)
}
```

Replace with:

```go
func newWorktreeForm(repoName string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Key("name").Title("branch").Placeholder("e.g. main, feature/my-thing").Validate(huh.ValidateNotEmpty()),
		).Title(fmt.Sprintf("%s – create or checkout branch", repoName)),
	)
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add app.go
git commit -m "feat: update branch form labels for create/checkout UX"
```

---

### Task 4: Manual smoke test

- [ ] **Step 1: Build and run**

```bash
go build -o tcr . && ./tcr start
```

- [ ] **Verify:** The project list loads. Pressing `c` on an existing project shows the "create or checkout branch" form with a "branch" input and placeholder `e.g. main, feature/my-thing`.

- [ ] **Verify:** Entering a branch name that exists on the remote checks it out. Entering a new name creates a new local branch. The worktree list updates to show local branches.

- [ ] **Verify:** Pressing `d` on a branch (not the current one) removes it from the list.

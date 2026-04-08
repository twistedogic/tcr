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
		project *Project
		err     error
	}

	ch := make(chan result, len(entries))
	count := 0
	sem := make(chan struct{}, maxConcurrency)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		count++
		dirName := entry.Name()
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			p, err := LoadProject(ctx, filepath.Join(workspace, dirName))
			ch <- result{project: p, err: err}
		}()
	}

	projects := make([]*Project, 0, count)
	for range count {
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

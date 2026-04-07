package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
)

const maxConcurrency = 4

type Worktree struct {
	Owner string
	Repo  string
	Name  string
	Path  string
}

func (w *Worktree) refresh() error {
	return nil
}

// implements list.Item
func (w *Worktree) Title() string       { return fmt.Sprintf("%s/%s – %s", w.Owner, w.Repo, w.Name) }
func (w *Worktree) Description() string { return "" }
func (w *Worktree) FilterValue() string { return w.Name }

func compareWorktree(a, b *Worktree) int { return cmp.Compare(a.Name, b.Name) }

type Project struct {
	repo  string
	owner string

	path      string
	worktrees []*Worktree
}

// implements list.Item
func (p *Project) Title() string       { return fmt.Sprintf("%s/%s", p.owner, p.repo) }
func (p *Project) Description() string { return fmt.Sprintf("%d branches", len(p.worktrees)) }
func (p *Project) FilterValue() string { return p.Title() }

func (p *Project) AddWorktree(ctx context.Context, name string) error {
	if err := os.MkdirAll(p.path, 0755); err != nil {
		return err
	}
	if err := createBranch(ctx, p.path, p.owner, p.repo, name); err != nil {
		return err
	}
	wt := &Worktree{Name: name, Path: filepath.Join(p.path, name), Owner: p.owner, Repo: p.repo}
	if idx, exist := slices.BinarySearchFunc(p.worktrees, wt, compareWorktree); exist {
		p.worktrees[idx] = wt
	} else {
		p.worktrees = append(p.worktrees, wt)
		slices.SortFunc(p.worktrees, compareWorktree)
	}
	return nil
}

func (p *Project) Refresh(ctx context.Context) error {
	entries, err := os.ReadDir(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			p.worktrees = nil
			return nil
		}
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	p.worktrees = make([]*Worktree, len(entries))

	idx := 0
	for _, entry := range entries {
		if entry.IsDir() {
			currentIdx := idx
			dirName := entry.Name()
			g.Go(func() error {
				wt := &Worktree{
					Name:  dirName,
					Path:  filepath.Join(p.path, dirName),
					Owner: p.owner,
					Repo:  p.repo,
				}
				p.worktrees[currentIdx] = wt
				return nil
			})
			idx++
		}
	}

	if err := g.Wait(); err != nil {
		return err
	}

	result := make([]*Worktree, 0, len(p.worktrees))
	for _, wt := range p.worktrees {
		if wt != nil {
			result = append(result, wt)
		}
	}
	p.worktrees = result
	slices.SortFunc(p.worktrees, compareWorktree)
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

// LoadProject loads a project from workspace/<repo> by reading the remote
// origin of any branch checkout found inside it.
func LoadProject(ctx context.Context, path string) (*Project, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var owner, repo string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		branchPath := filepath.Join(path, entry.Name())
		b, err := execute(ctx, branchPath, "git", "remote", "get-url", "origin")
		if err != nil {
			continue
		}
		owner, repo, err = parseOrigin(strings.TrimSpace(string(b)))
		if err != nil {
			continue
		}
		break
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("could not determine repo origin from %s", path)
	}

	p := &Project{
		owner: owner,
		repo:  repo,
		path:  path,
	}
	return p, p.Refresh(ctx)
}

func (p *Project) DeleteWorktree(ctx context.Context, name string) error {
	path := filepath.Join(p.path, name)
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	idx, found := slices.BinarySearchFunc(p.worktrees, &Worktree{Name: name}, compareWorktree)
	if found {
		p.worktrees = append(p.worktrees[:idx], p.worktrees[idx+1:]...)
	}
	return nil
}

func LoadProjects(ctx context.Context, workspace string) ([]*Project, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	projects := make([]*Project, len(entries))

	idx := 0
	for _, entry := range entries {
		if entry.IsDir() {
			currentIdx := idx
			dirName := entry.Name()
			g.Go(func() error {
				p, err := LoadProject(ctx, filepath.Join(workspace, dirName))
				if err != nil {
					return err
				}
				projects[currentIdx] = p
				return nil
			})
			idx++
		}
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	result := make([]*Project, 0, len(projects))
	for _, p := range projects {
		if p != nil {
			result = append(result, p)
		}
	}

	return result, nil
}

// LoadWorkspace is an alias for LoadProjects for backward compatibility.
// Deprecated: Use LoadProjects directly.
func LoadWorkspace(ctx context.Context, workspace string) ([]*Project, error) {
	return LoadProjects(ctx, workspace)
}

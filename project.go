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
	Owner  string
	Repo   string
	Name   string
	Path   string
	Model  string
	Status *Status
}

func (w *Worktree) refresh(ctx context.Context) error {
	if changes, err := listChanges(ctx, w.Path); err == nil {
		w.Status = &Status{}
		if len(changes) > 0 {
			status, err := showChange(ctx, w.Path, changes[0])
			if err != nil {
				return err
			}
			w.Status = &status
		}
	}
	return nil
}

func (w *Worktree) review(ctx context.Context, client *GitHubPRClient) (bool, error) {
	prs, err := client.FetchBranchPRs(ctx, w.Owner, w.Repo, w.Name)
	if err != nil {
		return false, err
	}
	if len(prs) == 0 {
		return true, nil
	}
	pr := prs[0]
	comments, err := client.Comments(ctx, w.Owner, w.Repo, pr.Number)
	if _, err := ocPrompt(ctx, w.Path, w.Model, comments.String()); err != nil {
		return false, err
	}
	if err := amendCommit(ctx, w.Path); err != nil {
		return false, err
	}
	return false, push(ctx, w.Path)
}

// implements list.Item
func (w *Worktree) Title() string       { return w.Name }
func (w *Worktree) Description() string { return w.Status.String() }
func (w *Worktree) FilterValue() string { return w.Name }

func compareWorktree(a, b *Worktree) int { return cmp.Compare(a.Name, b.Name) }

type Project struct {
	repo  string
	owner string

	worktreePath string
	repoPath     string
	worktrees    []*Worktree
}

// implements list.Item
func (p *Project) Title() string       { return fmt.Sprintf("%s/%s", p.owner, p.repo) }
func (p *Project) Description() string { return fmt.Sprintf("%d worktrees", len(p.worktrees)) }
func (p *Project) FilterValue() string { return p.Title() }

func (p *Project) AddWorktree(ctx context.Context, name string) error {
	if err := os.MkdirAll(p.worktreePath, 0755); err != nil {
		return err
	}
	path := filepath.Join(p.worktreePath, name)
	if err := createWorktree(ctx, p.repoPath, path); err != nil {
		return err
	}
	wt := &Worktree{Name: name, Path: path}
	if err := wt.refresh(ctx); err != nil {
		return err
	}
	if idx, exist := slices.BinarySearchFunc(p.worktrees, wt, compareWorktree); exist {
		p.worktrees[idx] = wt
	} else {
		p.worktrees = append(p.worktrees, wt)
		slices.SortFunc(p.worktrees, compareWorktree)
	}
	return nil
}

func (p *Project) Refresh(ctx context.Context) error {
	entries, err := os.ReadDir(p.worktreePath)
	if err != nil {
		if os.IsNotExist(err) {
			p.worktrees = nil
			return nil
		}
		return err
	}
	p.worktrees = make([]*Worktree, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			wt := &Worktree{Name: entry.Name(), Path: filepath.Join(p.worktreePath, entry.Name())}
			if err := wt.refresh(ctx); err != nil {
				return err
			}
			p.worktrees = append(p.worktrees, wt)
		}
	}
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

func LoadProject(ctx context.Context, path, worktreeDir string) (*Project, error) {
	b, err := execute(ctx, path, "git", "remote", "get-url", "origin")
	if err != nil {
		return nil, err
	}
	owner, repo, err := parseOrigin(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, err
	}
	p := &Project{
		owner:        owner,
		repo:         repo,
		worktreePath: filepath.Join(worktreeDir, repo),
		repoPath:     path,
	}
	return p, p.Refresh(ctx)
}

func (p *Project) DeleteWorktree(ctx context.Context, name string) error {
	path := filepath.Join(p.worktreePath, name)
	if err := deleteWorktree(ctx, p.repoPath, path); err != nil {
		return err
	}
	// Remove from slice
	idx, found := slices.BinarySearchFunc(p.worktrees, &Worktree{Name: name}, compareWorktree)
	if found {
		p.worktrees = append(p.worktrees[:idx], p.worktrees[idx+1:]...)
	}
	return nil
}

func LoadProjects(ctx context.Context, repoDir, worktreeDir string) ([]*Project, error) {
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return nil, err
	}
	projects := make([]*Project, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			p, err := LoadProject(ctx, filepath.Join(repoDir, entry.Name()), worktreeDir)
			if err != nil {
				return nil, err
			}
			projects = append(projects, p)
		}
	}
	return projects, nil
}

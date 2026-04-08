# Branch Form & Checkout Architecture Design

**Date:** 2026-04-08  
**Status:** Approved

## Summary

Change how branches are managed within a project. Instead of each branch being a separate `git clone` in its own subdirectory (`workspace/repo/branch-name/`), a project has a single clone at `workspace/repo/` and branches are managed via `git checkout`.

## Current Architecture

```
workspace/
  repo/
    main/          ← full git clone
    feature-foo/   ← full git clone
```

Each "worktree" is an independent clone. Creating a branch = cloning the remote into a new subdirectory.

## New Architecture

```
workspace/
  repo/            ← single git clone
```

A project is a single repo. Branches are checked out in place. Creating/switching a branch = `git fetch && git checkout <branch>` (or `git checkout -b <branch>` for new ones).

## Changes Required

### `git.go`

- Replace `createBranch` (which does `git clone --branch <name>`) with:
  - `checkoutBranch(ctx, repoPath, branch)` — runs `git fetch` then `git checkout <branch>` (checks out existing remote branch or creates new one)
- `clone` clones the repo into `workspace/repo/` directly (no branch subdirectory), checking out the initial branch

### `project.go`

- `Project.path` now points to the single repo root (e.g. `workspace/repo/`)
- `Project.AddWorktree` → calls `checkoutBranch` instead of `createBranch`
- `Project.Refresh` → reads branches from `git branch` output instead of subdirectories
- `Project.DeleteWorktree` → runs `git branch -d <name>` instead of `os.RemoveAll`
- `Worktree.Path` → always the repo root (`project.path`)
- `LoadProject` → reads `git remote get-url origin` from the repo root directly

### `app.go` — `newWorktreeForm`

- **Form group title**: `"<repo> – create or checkout branch"`
- **Input field title**: `"branch"`
- **Input placeholder**: `"e.g. main, feature/my-thing"`

## Rationale

A single checkout is simpler, uses less disk, and matches how most developers work. The existing multi-clone approach was designed for git worktrees but never used `git worktree add` — it was always independent clones. Using a single repo with branch switching is more conventional and avoids duplicating `.git` objects across branches.

## Edge Cases

- If a branch doesn't exist on remote, `git checkout -b <name>` creates it locally
- Deleting the currently checked-out branch should be prevented (show error)
- `LoadProject` no longer needs to iterate subdirectories to find the origin — reads from repo root directly

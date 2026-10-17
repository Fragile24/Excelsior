package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"excelsior/pkg/protocol"
)

func gitRoot(ctx context.Context, workspace string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", workspace, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("workspace is not a Git checkout: %w", err)
	}
	root, err := filepath.Abs(strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("resolve Git checkout path: %w", err)
	}
	return filepath.Clean(root), nil
}

func listWorktrees(ctx context.Context, workspace string) ([]protocol.WorktreeInfo, error) {
	root, err := gitRoot(ctx, workspace)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return []protocol.WorktreeInfo{}, nil
		}
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("list Git worktrees: %w", err)
	}

	current := cleanPath(root)
	var worktrees []protocol.WorktreeInfo
	for _, record := range strings.Split(strings.TrimSpace(string(out)), "\n\n") {
		if strings.TrimSpace(record) == "" {
			continue
		}
		var item protocol.WorktreeInfo
		bare := false
		prunable := false
		for _, line := range strings.Split(record, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				item.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "HEAD "):
				item.Head = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch "):
				item.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
			case line == "bare":
				bare = true
			case strings.HasPrefix(line, "prunable "):
				prunable = true
			}
		}
		if item.Path == "" || bare || prunable {
			continue
		}
		info, statErr := os.Stat(item.Path)
		if statErr != nil || !info.IsDir() {
			continue
		}
		item.Path = filepath.Clean(item.Path)
		item.Current = samePath(item.Path, current)
		if len(worktrees) == 0 {
			item.Main = true
		}
		worktrees = append(worktrees, item)
	}
	return worktrees, nil
}

func createWorktree(ctx context.Context, workspace, branch string) (protocol.WorktreeInfo, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.HasPrefix(branch, "-") {
		return protocol.WorktreeInfo{}, errors.New("enter a valid new branch name")
	}
	root, err := gitRoot(ctx, workspace)
	if err != nil {
		return protocol.WorktreeInfo{}, err
	}
	checkCtx, cancelCheck := context.WithTimeout(ctx, 5*time.Second)
	output, err := exec.CommandContext(checkCtx, "git", "-C", root, "check-ref-format", "--branch", branch).CombinedOutput()
	cancelCheck()
	if err != nil {
		return protocol.WorktreeInfo{}, fmt.Errorf("invalid branch name %q: %s", branch, strings.TrimSpace(string(output)))
	}

	base := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-worktrees")
	target := filepath.Join(base, filepath.FromSlash(branch))
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return protocol.WorktreeInfo{}, errors.New("branch name resolves outside the worktree directory")
	}
	if _, err := os.Lstat(target); err == nil {
		return protocol.WorktreeInfo{}, fmt.Errorf("worktree path already exists: %s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return protocol.WorktreeInfo{}, fmt.Errorf("check worktree path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return protocol.WorktreeInfo{}, fmt.Errorf("prepare worktree directory: %w", err)
	}
	worktreeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	output, err = exec.CommandContext(worktreeCtx, "git", "-C", root, "worktree", "add", "-b", branch, target, "HEAD").CombinedOutput()
	if err != nil {
		return protocol.WorktreeInfo{}, fmt.Errorf("create worktree: %s", strings.TrimSpace(string(output)))
	}
	return protocol.WorktreeInfo{Path: filepath.Clean(target), Branch: branch}, nil
}

func cleanPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func samePath(left, right string) bool {
	left = cleanPath(left)
	right = cleanPath(right)
	if a, err := os.Stat(left); err == nil {
		if b, err := os.Stat(right); err == nil && os.SameFile(a, b) {
			return true
		}
	}
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}

package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreeListAndCreate(t *testing.T) {
	repo := initGitRepo(t)
	ctx := context.Background()

	initial, err := listWorktrees(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial) != 1 || !initial[0].Main || !initial[0].Current || initial[0].Branch != "main" {
		t.Fatalf("unexpected initial worktree list: %#v", initial)
	}

	created, err := createWorktree(ctx, repo, "feature/context-meter")
	if err != nil {
		t.Fatal(err)
	}
	if created.Branch != "feature/context-meter" {
		t.Fatalf("created branch = %q", created.Branch)
	}
	if _, err := os.Stat(filepath.Join(created.Path, "README.md")); err != nil {
		t.Fatalf("new worktree is missing committed files: %v", err)
	}

	listed, err := listWorktrees(ctx, created.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d worktrees, want 2: %#v", len(listed), listed)
	}
	foundCurrent := false
	foundMain := false
	for _, item := range listed {
		if item.Path == created.Path && item.Branch == created.Branch && item.Current {
			foundCurrent = true
		}
		if item.Path == repo && item.Branch == "main" && item.Main && !item.Current {
			foundMain = true
		}
	}
	if !foundCurrent || !foundMain {
		t.Fatalf("worktree metadata did not track active and main checkouts: %#v", listed)
	}
}

func TestWorktreeCreationRejectsInvalidAndExistingPaths(t *testing.T) {
	repo := initGitRepo(t)
	if _, err := createWorktree(context.Background(), repo, "../escape"); err == nil {
		t.Fatal("invalid branch name was accepted")
	}
	if _, err := createWorktree(context.Background(), repo, "main"); err == nil {
		t.Fatal("existing branch was accepted as a new branch")
	}
	target := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-worktrees", filepath.FromSlash("feature/occupied"))
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := createWorktree(context.Background(), repo, "feature/occupied"); err == nil {
		t.Fatal("existing worktree path was accepted")
	}
	if contents, err := os.ReadFile(marker); err != nil || string(contents) != "keep" {
		t.Fatalf("existing path was changed: contents=%q err=%v", contents, err)
	}
}

func TestWorktreeListIgnoresNonGitFolders(t *testing.T) {
	workspace := t.TempDir()
	items, err := listWorktrees(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("listed worktrees for a non-Git folder: %#v", items)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, repo, "init", "--initial-branch=main")
	gitTestCommand(t, repo, "config", "user.name", "Excelsior Test")
	gitTestCommand(t, repo, "config", "user.email", "excelsior-test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, repo, "add", "README.md")
	gitTestCommand(t, repo, "commit", "-m", "initial commit")
	return repo
}

func gitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

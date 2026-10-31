package repo_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

// repos makes a git repo with a commit, and a linked worktree next to it. It returns the folders
// of the main worktree and of the linked one. git runs without the user's config.
func repos(t *testing.T) (mainDir, linked string) {
	t.Helper()

	dir := t.TempDir()
	mainDir, linked = filepath.Join(dir, "main"), filepath.Join(dir, "linked")

	if err := os.Mkdir(mainDir, 0o700); err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	for _, cmd := range []*exec.Cmd{
		exec.CommandContext(ctx, "git", "init", "--quiet"),
		exec.CommandContext(ctx, "git", "-c", "user.name=baton", "-c", "user.email=baton@example.com",
			"commit", "--quiet", "--allow-empty", "--message", "Start"),
		exec.CommandContext(ctx, "git", "worktree", "add", "--quiet", "../linked"),
	} {
		cmd.Dir = mainDir

		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", cmd, err, out)
		}
	}

	return mainDir, linked
}

func TestCommonDirIsTheSameInEveryWorktree(t *testing.T) {
	mainDir, linked := repos(t)

	sub := filepath.Join(mainDir, "docs", "notes")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	want, err := filepath.EvalSymlinks(filepath.Join(mainDir, ".git"))
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{mainDir, sub, linked} {
		got, err := repo.CommonDir(t.Context(), dir)
		if err != nil || got != want {
			t.Errorf("CommonDir(%s) = %q, %v; want %q", dir, got, err, want)
		}
	}
}

func TestCommonDirRefusesAFolderOutsideARepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("LC_ALL", "C")

	_, err := repo.CommonDir(t.Context(), dir)
	if err == nil {
		t.Fatal("CommonDir = nil error; want git's reason")
	}

	want := "find the git repo: fatal: not a git repository"
	if got := err.Error(); !strings.HasPrefix(got, want) || strings.HasSuffix(got, "\n") {
		t.Errorf("CommonDir = %q; want %q, then the rest of git's reason, trimmed", got, want)
	}
}

func TestCommonDirReportsAGitThatFailsWithoutAReason(t *testing.T) {
	bin, executable := t.TempDir(), os.FileMode(0o700)

	err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 3\n"), executable)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin)

	_, err = repo.CommonDir(t.Context(), t.TempDir())
	if err == nil || err.Error() != "find the git repo: exit status 3" {
		t.Errorf("CommonDir = %v; want git's exit status", err)
	}
}

func TestCommonDirReportsAMissingGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := repo.CommonDir(t.Context(), t.TempDir())
	if !errors.Is(err, exec.ErrNotFound) || !strings.HasPrefix(err.Error(), "find the git repo: ") {
		t.Errorf("CommonDir = %v; want git not found", err)
	}
}

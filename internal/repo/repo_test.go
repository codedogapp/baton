package repo_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

// TestMain runs git without the user's config, in the C locale, with a test author. It clears the
// variables that git sets for its hooks, such as GIT_DIR, which would point the tests' git at the
// repo that runs the hook.
func TestMain(m *testing.M) {
	var errs []error

	for _, env := range os.Environ() {
		if name, _, _ := strings.Cut(env, "="); strings.HasPrefix(name, "GIT_") {
			errs = append(errs, os.Unsetenv(name))
		}
	}

	for _, env := range []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C",
		"GIT_AUTHOR_NAME=baton", "GIT_AUTHOR_EMAIL=baton@example.com",
		"GIT_COMMITTER_NAME=baton", "GIT_COMMITTER_EMAIL=baton@example.com",
	} {
		name, value, _ := strings.Cut(env, "=")
		errs = append(errs, os.Setenv(name, value))
	}

	if err := errors.Join(errs...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// run runs git in the folder and returns what it printed, without the last newlines.
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()

	//nolint:gosec // G204: the tests' own git and arguments (decision 34).
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, exit.Stderr)
	}

	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}

	return strings.TrimRight(string(out), "\n")
}

// repos makes a git repo with a commit, and a linked worktree next to it. It returns the folders
// of the main worktree and of the linked one.
func repos(t *testing.T) (mainDir, linked string) {
	t.Helper()

	dir := t.TempDir()
	mainDir, linked = filepath.Join(dir, "main"), filepath.Join(dir, "linked")

	if err := os.Mkdir(mainDir, 0o700); err != nil {
		t.Fatal(err)
	}

	run(t, mainDir, "init", "--quiet")
	run(t, mainDir, "commit", "--quiet", "--allow-empty", "--message", "Start")
	run(t, mainDir, "worktree", "add", "--quiet", "../linked")

	return mainDir, linked
}

// script writes the shell script to an executable file in a folder of the test's, and returns
// the folder.
func script(t *testing.T, name, text string) string {
	t.Helper()

	dir, executable := t.TempDir(), os.FileMode(0o700)

	err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+text), executable)
	if err != nil {
		t.Fatal(err)
	}

	return dir
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
	t.Setenv("PATH", script(t, "git", "exit 3\n"))

	_, err := repo.CommonDir(t.Context(), t.TempDir())
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

func TestMainWorktreeIsTheSameFromEveryWorktree(t *testing.T) {
	mainDir, linked := repos(t)

	sub := filepath.Join(linked, "docs")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	want, err := filepath.EvalSymlinks(mainDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{mainDir, linked, sub} {
		got, err := repo.MainWorktree(t.Context(), dir)
		if err != nil || got != want {
			t.Errorf("MainWorktree(%s) = %q, %v; want %q", dir, got, err, want)
		}
	}
}

func TestMainWorktreeRefusesAFolderOutsideARepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	_, err := repo.MainWorktree(t.Context(), dir)
	if want := "find the main worktree: fatal: not a git repository"; err == nil ||
		!strings.HasPrefix(err.Error(), want) {
		t.Errorf("MainWorktree = %v; want %q, then the rest of git's reason", err, want)
	}
}

func TestMainWorktreeRefusesAListWithoutAWorktree(t *testing.T) {
	t.Setenv("PATH", script(t, "git", "echo nonsense\n"))

	_, err := repo.MainWorktree(t.Context(), t.TempDir())
	if want := `find the main worktree: git listed "nonsense\n" first`; fmt.Sprint(err) != want {
		t.Errorf("MainWorktree = %v; want %s", err, want)
	}
}

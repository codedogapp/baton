// Package repo runs git for baton: it finds the repo that baton works in, and makes a feature's
// branch and worktree.
package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CommonDir returns the git common folder of the repo that holds the folder: the main worktree's
// .git folder, which all the repo's worktrees share (fact 21). An empty folder stands for the
// working folder.
func CommonDir(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("find the git repo: %w", err)
	}

	return strings.TrimSuffix(out, "\n"), nil
}

// MainWorktree returns the folder of the main worktree of the repo that holds the folder, or a bare
// repo's folder, with symlinks resolved: the first worktree that git lists (fact 23).
func MainWorktree(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", fmt.Errorf("find the main worktree: %w", err)
	}

	first, _, _ := strings.Cut(out, "\x00")

	path, ok := strings.CutPrefix(first, "worktree ")
	if !ok {
		return "", fmt.Errorf("find the main worktree: git listed %q first", first)
	}

	return path, nil
}

// git runs git in the folder and returns what it printed.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	// Sonar's go:S4036 wants git at a fixed path, but baton runs the user's own git, found on the
	// user's PATH as the user's shell finds it (decision 27). gosec's G204 wants constant
	// arguments, but git needs the feature's own; they reach it as arguments, which no shell reads
	// (decision 34).
	//nolint:gosec // G204: the caller's names come after --end-of-options (fact 23).
	cmd := exec.CommandContext(ctx, "git", args...) // NOSONAR
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return "", failure(ctx, err)
	}

	return string(out), nil
}

// failure says why running git failed.
func failure(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}

	if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
		return errors.New(string(bytes.TrimSpace(exit.Stderr)))
	}

	return err
}

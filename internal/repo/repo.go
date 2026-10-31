// Package repo finds the git repo that baton works in.
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
	// Sonar's go:S4036 wants git at a fixed path, but baton runs the user's own git, found on the
	// user's PATH as the user's shell finds it (decision 27).
	cmd := exec.CommandContext( // NOSONAR
		ctx,
		"git",
		"rev-parse",
		"--path-format=absolute",
		"--git-common-dir",
	)
	cmd.Dir = dir

	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
		return "", fmt.Errorf("find the git repo: %s", bytes.TrimSpace(exit.Stderr))
	}

	if err != nil {
		return "", fmt.Errorf("find the git repo: %w", err)
	}

	return strings.TrimSuffix(string(out), "\n"), nil
}

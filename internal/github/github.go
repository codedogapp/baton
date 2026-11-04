// Package github runs gh, GitHub's command line tool, for baton deliver: it opens a feature's pull
// request and comments on it, as the user's own gh, with the user's login (decision 61).
package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// PullRequest is a pull request to open: from the branch, which origin has, with the title, and
// with the text of the body file.
type PullRequest struct{ Branch, Title, BodyFile string }

// Open opens the pull request in the GitHub repo of the repo that holds the folder, as gh finds it,
// into that repo's default branch, and returns its URL. gh pushes nothing, asks nothing and
// uploads nothing that the body links to (fact 43).
func Open(ctx context.Context, dir string, pr PullRequest) (string, error) {
	out, err := gh(ctx, dir, "pr", "create", "--head="+pr.Branch, "--title="+pr.Title,
		"--body-file="+pr.BodyFile)
	if err != nil {
		return "", fmt.Errorf("open the pull request: %w", err)
	}

	return strings.TrimSpace(out), nil
}

// Comment comments on the pull request at the URL, with the text of the body file.
func Comment(ctx context.Context, dir, url, bodyFile string) error {
	if _, err := gh(ctx, dir, "pr", "comment", "--body-file="+bodyFile, "--", url); err != nil {
		return fmt.Errorf("comment on the pull request %s: %w", url, err)
	}

	return nil
}

// gh runs gh in the folder and returns what it printed. Its error holds what gh printed on stderr,
// if gh failed and said why.
func gh(ctx context.Context, dir string, args ...string) (string, error) {
	// Sonar's go:S4036 wants gh at a fixed path, but baton runs the user's own gh, found on the
	// user's PATH as the user's shell finds it (decision 27). gosec's G204 wants constant
	// arguments, but gh needs the feature's own; they reach it as arguments, which no shell reads,
	// after an = or a -- (decision 34).
	//nolint:gosec // G204: the caller's values come after an = or a -- (decision 61).
	cmd := exec.CommandContext(ctx, "gh", args...) // NOSONAR
	cmd.Dir = dir

	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
		return "", fmt.Errorf("%s (%w)", bytes.TrimSpace(exit.Stderr), err)
	}

	if err != nil {
		return "", err
	}

	return string(out), nil
}

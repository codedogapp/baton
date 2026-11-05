package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// restore starts again the teams of the repo's open features (decision 69): it makes again the
// tmux session of each one whose session is missing, as after a restart of the Mac, and starts
// again the panes of a running session whose copilot ended, which the session kept. It tries every
// feature, then fails if any failed, so that the user reads why before tmux shows the
// orchestrator's session, which would hide it.
func restore(ctx context.Context, env Env, mainDir string) error {
	self, err := env.Executable()
	if err != nil {
		return fmt.Errorf("find baton's own program: %w", err)
	}

	store, err := openStore(ctx, mainDir)

	var features []state.Feature
	if err == nil {
		features, err = store.List()
	}

	if err != nil {
		return err
	}

	var failed []error

	for _, feature := range features {
		if feature.Flow.Status == flow.StatusClosed {
			continue
		}

		if err := restoreTeam(ctx, env, self, feature); err != nil {
			failed = append(failed, fmt.Errorf("start %s's team again: %w", feature.Slug, err))
		}
	}

	return errors.Join(failed...)
}

// restoreTeam starts the feature's team again: the whole tmux session if it isn't there, or else
// the session's dead panes. It says which panes it started.
func restoreTeam(ctx context.Context, env Env, self string, feature state.Feature) error {
	session := teamSession(self, feature.Slug, feature.Worktree)

	running, err := teamRuns(ctx, env.Tmux, feature)
	if err != nil {
		return err
	}

	var started []string

	if running {
		started, err = env.Tmux.Respawn(ctx, session)
	} else if err = env.Tmux.NewSession(ctx, session); err == nil {
		for _, pane := range session.Panes {
			started = append(started, pane.Title)
		}
	}

	if len(started) > 0 {
		_, _ = fmt.Fprintf(env.Stdout, "Started again in tmux session %s: %s.\n", session.Name,
			strings.Join(started, ", "))
	}

	return err
}

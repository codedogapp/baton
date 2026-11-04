package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/repo"
	"github.com/codedogapp/baton/internal/state"
)

// closeFeature closes a feature on the user's word, whatever its status: it records the feature
// closed, which nothing moves again, then ends its tmux session, with the team's panes, even
// mid-work, and removes its worktree if asked, but keeps its branch (decision 63). For a feature
// that is closed already, it does the rest again, so that a close that failed halfway can run
// again. Only the orchestrator closes features, so it refuses a role's pane.
func closeFeature(ctx context.Context, env Env, args []string) error {
	slug, removing, err := parseClose(env, args)
	if err != nil {
		return err
	}

	if role := flow.Role(env.Getenv(envRole)); role != "" && role != flow.RoleOrchestrator {
		return fmt.Errorf("only the orchestrator closes a feature, and %s names the %s", envRole,
			role)
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	end := closing{removing: removing}

	end.feature, err = store.Read(slug)
	if err == nil {
		end.running, err = teamRuns(ctx, env.Tmux, end.feature)
	}

	event := state.Event{Kind: state.EventClose, Role: flow.RoleOrchestrator}
	if err == nil && end.feature.Flow.Status != flow.StatusClosed {
		_, err = store.Update(slug, state.Entry{Event: event}, closed)
	}

	if err != nil {
		return err
	}

	return end.finish(ctx, env)
}

// parseClose parses baton feature close's command line: the feature's slug, and whether to remove
// its worktree too.
func parseClose(env Env, args []string) (slug string, removing bool, err error) {
	flags := newFlags(env, "feature close")
	remove := flags.Bool("remove-worktree", false, "remove the feature's worktree too, which git "+
		"refuses while it holds changes that no commit has")

	slug, err = parseSlug(flags, args)
	if err == nil && slug == "" {
		err = errors.New("want the feature's slug: baton feature close <slug> [--remove-worktree]")
	}

	return slug, *remove, err
}

// teamRuns reports whether the feature's tmux session runs. It refuses a session of that name in
// another folder than the feature's worktree, which baton didn't make for the feature.
func teamRuns(ctx context.Context, sessions Server, feature state.Feature) (bool, error) {
	name := featureSession(feature.Worktree)

	dir, found := sessions.SessionDir(ctx, name)
	if found && dir != feature.Worktree {
		return false, fmt.Errorf("tmux session %s runs in %s, not in the worktree of %s, %s: "+
			"baton ends only the session that it made", name, dir, feature.Slug, feature.Worktree)
	}

	return found, nil
}

// closed closes the feature, which leaves no one work: no note to pick up, and no question to
// answer.
func closed(feature *state.Feature) error {
	next, err := feature.Flow.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", feature.Slug, err)
	}

	feature.Flow, feature.Work, feature.Question = next, state.Work{}, state.Question{}

	return nil
}

// closing is what baton feature close ends once the feature is closed: its tmux session, if it
// runs, and its worktree, if asked.
type closing struct {
	feature  state.Feature
	running  bool // Whether the feature's tmux session runs.
	removing bool // Whether to remove the feature's worktree.
}

// finish ends the closed feature's tmux session and removes its worktree, then says what it did.
func (end closing) finish(ctx context.Context, env Env) error {
	feature := end.feature
	name := featureSession(feature.Worktree)
	ended := "tmux session " + name + " had ended"

	if end.running {
		if err := env.Tmux.KillSession(ctx, name); err != nil {
			return fmt.Errorf("%s is closed, but its tmux session runs: %w", feature.Slug, err)
		}

		ended = "ended tmux session " + name
	}

	kept := "kept branch " + feature.Branch + " and worktree " + feature.Worktree
	if end.removing {
		worktree := repo.Worktree{Path: feature.Worktree, Branch: feature.Branch}
		if err := worktree.Remove(ctx, env.Dir); err != nil {
			return fmt.Errorf("%s is closed, and %s, but its worktree stays: %w", feature.Slug,
				ended, err)
		}

		kept = "removed worktree " + feature.Worktree + " and kept branch " + feature.Branch
	}

	_, err := fmt.Fprintf(env.Stdout, "Closed %s: %s; %s.\n", feature.Slug, ended, kept)

	return err
}

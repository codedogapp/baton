package cli

import (
	"context"
	"fmt"

	"github.com/codedogapp/baton/internal/config"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/gate"
	"github.com/codedogapp/baton/internal/state"
)

// handing is a role's handover: the feature, in its store, the move in its flow, and the event,
// with the note, that it adds to the feature's journal.
type handing struct {
	store state.Store
	slug  string
	move  flow.Handover
	entry state.Entry
}

// make makes the move, once the repo's gate passes in the feature's worktree if the move waits for
// it, and returns the feature as moved. It checks the move first, so that a move that the flow
// refuses runs no gate, and the store checks it again under the feature's lock, which the gate
// doesn't hold, since it can take minutes (decision 56).
func (h handing) make(ctx context.Context, env Env) (state.Feature, error) {
	feature, err := h.store.Read(h.slug)
	if err == nil {
		err = moved(h.move)(&feature)
	}

	if err == nil && feature.Flow.Gated() {
		h.entry.Event.Gate, err = runGate(ctx, env, feature)
	}

	if err != nil {
		return state.Feature{}, err
	}

	return h.store.Update(h.slug, h.entry, moved(h.move))
}

// runGate runs the repo's gate in the feature's worktree, with its output on baton's, and returns
// the gate once it passes.
func runGate(ctx context.Context, env Env, feature state.Feature) (string, error) {
	settings, err := gatedSettings(ctx, env.Dir)
	if err == nil && feature.Worktree == "" {
		err = fmt.Errorf("feature %s names no worktree, where its gate runs", feature.Slug)
	}

	if err != nil {
		return "", err
	}

	command := settings.Gate
	_, _ = fmt.Fprintf(env.Stdout, "Running the gate in %s: %s\n", feature.Worktree, command)

	if err := gate.Run(ctx, feature.Worktree, command, env.Stdout); err != nil {
		return "", fmt.Errorf("gate %q failed: %w: the feature stays with you until it passes",
			command, err)
	}

	return command, nil
}

// gatedSettings returns the repo's settings, as loadSettings does, and refuses a repo without a
// gate, which each feature's handovers run (decision 56).
func gatedSettings(ctx context.Context, dir string) (config.Settings, error) {
	settings, err := loadSettings(ctx, dir)
	if err == nil && settings.Gate == "" {
		err = noGate()
	}

	return settings, err
}

// noGate returns the error for a repo without a gate, which names the config file where it goes.
func noGate() error {
	return fmt.Errorf(`no gate for this repo: set "gate" in %s to the command that checks `+
		`the repo's code, such as "mise run check", or to "true" for none`, configFile())
}

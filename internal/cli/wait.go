package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// poll is how often baton wait looks for the role's work (decision 21).
const poll = time.Second

// errNoWork means that another role holds the feature, so the role has no work in it yet.
var errNoWork = errors.New("no work yet")

// wait waits until the pane's role has work in its feature, then picks it up: it marks the work
// picked up and logs the pickup, under the feature's lock, then prints the work (decisions 12 and
// 31).
func wait(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "wait"), args); err != nil {
		return err
	}

	role, slug, err := pane(env.Getenv)
	if err != nil {
		return err
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	feature, err := pickUp(ctx, store, role, slug)
	if err != nil {
		return err
	}

	now, work := feature.Flow, feature.Work

	from := string(work.From)
	if work.Verdict != flow.VerdictNone {
		from += ", verdict " + string(work.Verdict)
	}

	_, err = fmt.Fprintf(env.Stdout, "Picked up %s as %s: round %d of %d, from %s.\nNote: %s\n",
		slug, role, now.Round, now.Cap+now.Extra, from, store.NotePath(slug, work.Note))

	return err
}

// pickUp picks up the role's work in the feature as soon as the role has some, and returns the
// feature. It tries every second, each time under the feature's lock, so two waits never pick up
// the same work.
func pickUp(ctx context.Context, store state.Store, role flow.Role, slug string) (
	state.Feature, error,
) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	entry := state.Entry{Event: state.Event{Kind: state.EventPickup, Role: role}}

	for {
		feature, err := store.Update(slug, entry, picked(role))
		if !errors.Is(err, errNoWork) {
			return feature, err
		}

		select {
		case <-ctx.Done():
			return state.Feature{}, fmt.Errorf("stopped waiting: %w", context.Cause(ctx))
		case <-ticker.C:
		}
	}
}

// picked returns the change that marks the role's work picked up. It fails with errNoWork while
// another role holds the feature. A role that holds it with no work left to pick up gets a
// reason instead, since no work can come to it before it hands the feature over.
func picked(role flow.Role) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		switch work := &feature.Work; {
		case feature.Flow.Holder != role:
			return errNoWork
		case work.Note == "" || work.Picked:
			return fmt.Errorf(
				"%s holds %s, with no work left to pick up: hand it over before waiting",
				role, feature.Slug)
		default:
			work.Picked = true

			return nil
		}
	}
}

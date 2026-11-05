package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"uuid"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// firstTurn is the first turn of a role's copilot: its session's ID, and its prompt, the role's
// first work.
type firstTurn struct {
	session uuid.UUID
	work    string
}

// carryOn starts the first prompt of a role's copilot whose pane started again while the role held
// the work that it picked up (decision 67).
const carryOn = "Your pane started again: carry on with this work.\n"

// firstWork returns the first turn of the role's copilot. If the role holds the feature with its
// work picked up, as when its copilot ended or failed to start, the pane carries on with that work
// at once, and logs that it did (decision 67). Otherwise it says that the role waits, waits for
// the role's work in the feature, then picks it up, as baton wait does. Either way, the same
// change keeps a new session's ID as the role's, unless the role has one in the feature already
// (decision 66). While it waits, it marks the role as waiting, as baton wait does (decision 47),
// and drops what the user types in the pane; it stops waiting when the pane's terminal ends.
func firstWork(ctx context.Context, env Env, role flow.Role, slug string) (firstTurn, error) {
	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return firstTurn{}, err
	}

	session := uuid.New()

	switch feature, err := store.Update(slug, resumed(role), carryingOn(role, session)); {
	case err == nil:
		return turnIn(store, feature, role, carryOn), nil
	case !errors.Is(err, errNoWork):
		return firstTurn{}, err
	}

	if _, err := fmt.Fprintf(env.Stdout, "%s in %s: waiting for work.\n", role, slug); err != nil {
		return firstTurn{}, err
	}

	waiting, stop := context.WithCancelCause(ctx)
	defer stop(nil)

	go watch(env.Stdin, stop)

	feature, err := waitFor(
		waiting,
		func() (func(), error) { return store.MarkWaiting(slug, role) },
		func() (state.Feature, error) {
			return store.Update(slug, pickup(role), starting(role, session))
		},
	)
	if err != nil {
		return firstTurn{}, err
	}

	return turnIn(store, feature, role, ""), nil
}

// turnIn returns the first turn of the role's copilot in the feature: the role's session, and the
// role's work after the intro.
func turnIn(store state.Store, feature state.Feature, role flow.Role, intro string) firstTurn {
	return firstTurn{*feature.Sessions.Of(role), intro + pickupText(store, feature, role)}
}

// resumed is the journal's entry for a pane that carries on with its role's work.
func resumed(role flow.Role) state.Entry {
	return state.Entry{Event: state.Event{Kind: state.EventResume, Role: role}}
}

// carryingOn returns the change that lets the role carry on with its work, and keeps the session's
// ID as the role's, unless the role has one. It fails with errNoWork unless the role holds the
// feature with its work picked up. The orchestrator holds a closed feature, so no role carries on
// in one.
func carryingOn(role flow.Role, session uuid.UUID) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		if feature.Flow.Holder != role || !feature.Work.Picked {
			return errNoWork
		}

		keepSession(feature, role, session)

		return nil
	}
}

// starting returns the change that marks the role's work picked up, as picked does, and keeps the
// session's ID as the role's, unless the role has one.
func starting(role flow.Role, session uuid.UUID) func(*state.Feature) error {
	pick := picked(role)

	return func(feature *state.Feature) error {
		if err := pick(feature); err != nil {
			return err
		}

		keepSession(feature, role, session)

		return nil
	}
}

// keepSession keeps the session's ID in the feature as the role's, unless the role has one, so
// that a role has one session in a feature (decision 67).
func keepSession(feature *state.Feature, role flow.Role, session uuid.UUID) {
	kept := feature.Sessions.Of(role)
	*kept = cmp.Or(*kept, session)
}

// watch reads the pane's terminal, and drops what the user types there, until the terminal's
// input ends, then stops the wait. Once a terminal is gone, a read of it ends, even in a program
// that missed the hangup (facts 22 and 25). Ctrl-D ends the input too.
func watch(terminal io.Reader, stop context.CancelCauseFunc) {
	_, err := io.Copy(io.Discard, terminal)
	stop(fmt.Errorf("the terminal's input ended: %w", cmp.Or(err, io.EOF)))
}

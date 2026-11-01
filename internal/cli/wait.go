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

// errNoWork means that the role has no work in the feature yet.
var errNoWork = errors.New("no work yet")

// wait waits until the pane's role has work, then picks it up: it marks the work picked up and
// logs the pickup, under the feature's lock, then prints the work (decisions 12 and 31). A role
// waits for work in its feature, and the orchestrator in any feature (decision 44).
func wait(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "wait"), args); err != nil {
		return err
	}

	role, slug, err := fromPane(env.Getenv)
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

	_, err = fmt.Fprint(env.Stdout, pickupText(store, feature, role))

	return err
}

// pickupText says what work the role picked up in the feature: the round, the note's writer and
// their verdict, and the note's path. The orchestrator's work also says why it holds the feature:
// it's ready or escalated.
func pickupText(store state.Store, feature state.Feature, role flow.Role) string {
	now, work := feature.Flow, feature.Work

	stands := fmt.Sprintf("round %d of %d", now.Round, now.Cap+now.Extra)
	if now.Status != flow.StatusRunning {
		stands = string(now.Status) + ", " + stands
	}

	from := string(work.From)
	if work.Verdict != flow.VerdictNone {
		from += ", verdict " + string(work.Verdict)
	}

	return fmt.Sprintf("Picked up %s as %s: %s, from %s.\nNote: %s\n",
		feature.Slug, role, stands, from, store.NotePath(feature.Slug, work.Note))
}

// pickUp picks up the role's work as soon as the role has some, and returns its feature: a role's
// work in the feature, or the orchestrator's in any feature. It tries every second, each time
// under the feature's lock, so two waits never pick up the same work.
func pickUp(ctx context.Context, store state.Store, role flow.Role, slug string) (
	state.Feature, error,
) {
	try := func() (state.Feature, error) { return store.Update(slug, pickup(role), picked(role)) }
	if role == flow.RoleOrchestrator {
		try = func() (state.Feature, error) { return pickUpAny(store) }
	}

	tick := time.Tick(poll)

	for {
		feature, err := try()
		if !errors.Is(err, errNoWork) {
			return feature, err
		}

		select {
		case <-ctx.Done():
			return state.Feature{}, fmt.Errorf("stopped waiting: %w", context.Cause(ctx))
		case <-tick:
		}
	}
}

// pickUpAny picks up the orchestrator's work in the first feature, by slug, that has some. It
// fails with errNoWork if none has, as long as a role holds a feature, since work can come from
// that feature. Once the orchestrator holds them all, or there are none, no work can come, so it
// gives a reason instead (decision 44).
func pickUpAny(store state.Store) (state.Feature, error) {
	features, err := store.List()
	if err != nil {
		return state.Feature{}, err
	}

	running := false

	for _, feature := range features {
		if feature.Flow.Holder != flow.RoleOrchestrator {
			running = true

			continue
		}

		got, err := store.Update(feature.Slug, pickup(flow.RoleOrchestrator),
			picked(flow.RoleOrchestrator))
		if !errors.Is(err, errNoWork) {
			return got, err
		}
	}

	if !running {
		return state.Feature{}, errors.New(
			"no feature is running, so no work can come to the orchestrator")
	}

	return state.Feature{}, errNoWork
}

// pickup is the journal's entry for the role's pickup.
func pickup(role flow.Role) state.Entry {
	return state.Entry{Event: state.Event{Kind: state.EventPickup, Role: role}}
}

// picked returns the change that marks the role's work picked up. It fails with errNoWork while
// another role holds the feature. A role that holds it with no work left to pick up gets a
// reason instead, since no work can come to it before it hands the feature over. The orchestrator
// gets errNoWork then too: it holds the features that wait on the user, and looks for work in the
// others.
func picked(role flow.Role) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		switch work := &feature.Work; {
		case feature.Flow.Holder != role:
			return errNoWork
		case work.Note != "" && !work.Picked:
			work.Picked = true

			return nil
		case role == flow.RoleOrchestrator:
			return errNoWork
		default:
			return fmt.Errorf(
				"%s holds %s, with no work left to pick up: hand it over before waiting",
				role, feature.Slug)
		}
	}
}

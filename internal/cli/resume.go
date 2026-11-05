package cli

import (
	"cmp"
	"context"
	"errors"
	"uuid"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// sessionInput is what baton reads of the sessionStart hook's input: the ID of the session that
// starts, and the copilot's folder (fact 50).
type sessionInput struct {
	Session uuid.UUID `json:"sessionId"`
	Dir     string    `json:"cwd"`
}

// hookSession is the plugin's sessionStart hook, which runs at a new session's first prompt. In
// the orchestrator's pane, it keeps the session's ID as the one that the pane resumes when it
// starts again, since the user can start a new session there with /new or /clear. A role's pane
// gets its session from baton, so there the hook only reads its input, as it does first in every
// pane, so that copilot never writes it to a hook that has gone. Copilot goes on when the hook
// fails, so baton then reports why and exits with 1 (decision 68).
func hookSession(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "hook session"), args); err != nil {
		return err
	}

	var input sessionInput
	if err := readInput(env, &input); err != nil {
		return err
	}

	role, _, err := fromPane(env.Getenv)
	if err != nil || role != flow.RoleOrchestrator {
		return err
	}

	store, err := openStore(ctx, cmp.Or(input.Dir, env.Dir))
	if err != nil {
		return err
	}

	return store.KeepOrchestratorSession(input.Session)
}

// becomeOrchestrator becomes the orchestrator's copilot, in the session that baton keeps for it,
// once the pane has given back what its last copilot's waits picked up (decision 68).
func becomeOrchestrator(ctx context.Context, env Env, start copilotStart) error {
	session, err := resumeOrchestrator(ctx, env.Dir)
	if err != nil {
		return err
	}

	return becomeCopilot(env, start.program, orchestrating(start.plugin, start.own, session),
		environment(start.vars...))
}

// resumable checks what the orchestrator's pane reads in the repo's baton folder when it starts:
// the orchestrator's session and the features. A pane that fails takes its reason with it, so
// baton start checks them first, where the user sees why (risk 19).
func resumable(ctx context.Context, dir string) error {
	store, err := openStore(ctx, dir)
	if err == nil {
		_, err = store.OrchestratorSession()
	}

	if err == nil {
		_, err = store.List()
	}

	return err
}

// resumeOrchestrator returns the session for the orchestrator's copilot, which baton keeps. Then
// it gives the orchestrator back what its waits picked up that still waits on it, since their
// output went with the copilot that ran them, so that its next wait picks that up again (decision
// 68).
func resumeOrchestrator(ctx context.Context, dir string) (uuid.UUID, error) {
	store, err := openStore(ctx, dir)
	if err != nil {
		return uuid.Nil(), err
	}

	session, err := keptSession(store)
	if err != nil {
		return uuid.Nil(), err
	}

	features, err := store.List()
	if err != nil {
		return uuid.Nil(), err
	}

	for _, feature := range features {
		_, err := store.Update(feature.Slug, resumed(flow.RoleOrchestrator), unpicked)
		if err != nil && !errors.Is(err, errNoWork) {
			return uuid.Nil(), err
		}
	}

	return session, nil
}

// keptSession returns the session that baton keeps for the orchestrator's copilot, which copilot
// resumes, or else a new one, which baton keeps from then on.
func keptSession(store state.Store) (uuid.UUID, error) {
	session, err := store.OrchestratorSession()
	if err != nil || session != uuid.Nil() {
		return session, err
	}

	session = uuid.New()

	return session, store.KeepOrchestratorSession(session)
}

// unpicked gives the orchestrator back what its wait picked up in the feature that still waits on
// it: a role's question without its answer, or the work of a feature that is ready or escalated,
// which waits on the user's word. It fails with errNoWork if there is none.
func unpicked(feature *state.Feature) error {
	question, work := &feature.Question, &feature.Work

	switch status := feature.Flow.Status; {
	case question.Open() && question.Picked:
		question.Picked = false
	case (status == flow.StatusReady || status == flow.StatusEscalated) && work.Picked:
		work.Picked = false
	default:
		return errNoWork
	}

	return nil
}

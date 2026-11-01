package cli

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"slices"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The reasons that baton hook stop gives a copilot that ends its turn too soon, which copilot
// sends it as its next prompt (decision 47).
const (
	noWait = "No `baton wait` of yours is running, so no work can reach you. Start `baton wait` " +
		"with the bash tool in async mode, then end your turn: it ends, and wakes you, when your " +
		"next work comes, or at once if it's there already."
	askAgain = "Your question waits for its answer, but no `baton ask` of yours is running, so " +
		"the answer can't reach you. Ask it again, with the same text, with the bash tool in " +
		"async mode, then end your turn: baton doesn't ask it twice, and the ask ends, and wakes " +
		"you, when the answer comes. Your question:\n"
	handOver = "You hold the feature and haven't handed it over, and no `baton wait` of yours is " +
		"running. Finish your part and hand over with `baton handover`, then start `baton wait` " +
		"with the bash tool in async mode and end your turn. If you need an answer to go on, ask " +
		"the orchestrator with `baton ask` in async mode instead. If a command of yours runs in " +
		"the background, just end your turn: its end wakes you."
)

// stopInput is what baton reads of the agentStop hook's input: the copilot's folder, and whether
// the hook has sent it back to work in this turn already (fact 4).
type stopInput struct {
	Dir    string `json:"cwd"`
	Active bool   `json:"stop_hook_active"`
}

// stopOutput is the agentStop hook's answer that sends the copilot back to work, with the reason
// as its next prompt.
type stopOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// hookStop is the plugin's agentStop hook, which runs when a pane's copilot ends its turn. It lets
// the copilot stop while a baton wait or a baton ask of its role waits, since their end wakes it,
// and sends it back to work otherwise, with a reason that says what to do. It sends it back once a
// turn, so that a copilot that can't do what it says still stops. Copilot lets the copilot stop
// when the hook fails, so baton then reports why and exits with 1 (decision 47).
func hookStop(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "hook stop"), args); err != nil {
		return err
	}

	var input stopInput
	if err := json.UnmarshalDecode(jsontext.NewDecoder(env.Stdin), &input); err != nil {
		return fmt.Errorf("read the hook's input: %w", err)
	}

	if input.Active {
		return nil
	}

	role, slug, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	store, err := openStore(ctx, cmp.Or(input.Dir, env.Dir))
	if err != nil {
		return err
	}

	reason, err := stopReason(store, role, slug)
	if err != nil || reason == "" {
		return err
	}

	output := stopOutput{Decision: "block", Reason: reason}

	return json.MarshalEncode(jsontext.NewEncoder(env.Stdout), output)
}

// stopReason returns the reason that sends the role's copilot back to work, or "" to let it stop.
// A role that waits may stop. So may the orchestrator when no work can come to it. A role that
// doesn't wait gets the reason to wait if work can come to it, to ask again if its question waits
// for its answer, and to finish its part and hand over otherwise.
func stopReason(store state.Store, role flow.Role, slug string) (string, error) {
	waiting, err := store.Waiting(slug, role)
	if err != nil || waiting {
		return "", err
	}

	if role == flow.RoleOrchestrator {
		features, err := store.List()
		if err != nil || !slices.ContainsFunc(features, workComes(role)) {
			return "", err
		}

		return noWait, nil
	}

	feature, err := store.Read(slug)

	switch {
	case err != nil:
		return "", err
	case workComes(role)(feature):
		return noWait, nil
	case feature.Question.Open():
		return askAgain + feature.Question.Text, nil
	default:
		return handOver, nil
	}
}

// workComes returns whether work can come to the role in a feature, where its wait would wait, or
// is there already, where its wait would pick it up at once: another role holds the feature, or
// the role holds it with work that it hasn't picked up.
func workComes(role flow.Role) func(state.Feature) bool {
	return func(feature state.Feature) bool {
		work := feature.Work

		return feature.Flow.Holder != role || work.Note != "" && !work.Picked
	}
}

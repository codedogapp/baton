package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// handover hands a feature over from the pane's role to the next one, with a note: the next role's
// work. QA and the reviewer give their verdict. The orchestrator, whose pane names no feature,
// names the feature and the next role (decision 44).
func handover(ctx context.Context, env Env, args []string) error {
	flags := newFlags(env, "handover")
	notePath := flags.String("note", "", "the `file` with the note for the next role (required)")
	verdict := flags.String("verdict", "", "QA's or the reviewer's `verdict`: approve or reject")
	next := flags.String("to", "", "the orchestrator's pick of the next `role`: lead or engineer")

	named, err := parseSlug(flags, args)
	if err != nil {
		return err
	}

	role, slug, err := fromPane(env.Getenv)
	if err == nil {
		slug, err = handedOver(role, slug, named, *next)
	}

	if err != nil {
		return err
	}

	note, err := readNote(env.Dir, "note", *notePath)
	if err != nil {
		return err
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	move := flow.Handover{From: role, To: flow.Role(*next), Verdict: flow.Verdict(*verdict)}
	entry := state.Entry{
		Event: state.Event{Kind: state.EventHandover, Role: role, Verdict: move.Verdict},
		Note:  note,
	}

	feature, err := store.Update(slug, entry, moved(move))
	if err != nil {
		return err
	}

	now := feature.Flow
	_, _ = fmt.Fprintf(env.Stdout, "Handed %s over to %s: %s, round %d of %d.\n",
		slug, now.Holder, now.Status, now.Round, now.Cap+now.Extra)

	return nil
}

// handedOver returns the feature that the role hands over: a role's pane names it, and the
// orchestrator names it on the command line, with the next role, since its pane names no feature.
func handedOver(role flow.Role, slug, named, next string) (string, error) {
	switch {
	case role != flow.RoleOrchestrator && named != "":
		return "", fmt.Errorf("unexpected argument %q", named)
	case role != flow.RoleOrchestrator:
		return slug, nil
	case named == "":
		return "", errors.New(
			"want the feature's slug: baton handover <slug> --to <role> --note <file>")
	case next == "":
		return "", errors.New(
			"want --to lead, with a new brief, or, after an escalation, --to engineer")
	default:
		return named, nil
	}
}

// readNote reads the note in the file, whose path is relative to the working folder. The note is
// the next role's work, so it can't be blank. The name is the note's flag: note, or brief.
func readNote(dir, name, path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("want --%s <file>: the %s for the next role", name, name)
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	note, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read the %s: %w", name, err)
	}

	if len(bytes.TrimSpace(note)) == 0 {
		return nil, fmt.Errorf("%s %s is blank: it's the next role's work", name, path)
	}

	return note, nil
}

// moved returns the change that makes the move in the feature's flow.
func moved(move flow.Handover) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		next, err := feature.Flow.Next(move)
		if err != nil {
			return err
		}

		feature.Flow = next

		return nil
	}
}

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

// handover hands the pane's feature over from its role to the next one, with a note: the next
// role's work. QA and the reviewer give their verdict.
func handover(ctx context.Context, env Env, args []string) error {
	flags := newFlags(env, "handover")
	notePath := flags.String("note", "", "the `file` with the note for the next role (required)")
	verdict := flags.String("verdict", "", "QA's or the reviewer's `verdict`: approve or reject")

	if err := parse(flags, args); err != nil {
		return err
	}

	role, slug, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	note, err := readNote(env.Dir, *notePath)
	if err != nil {
		return err
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	move := flow.Handover{From: role, Verdict: flow.Verdict(*verdict)}
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

// readNote reads the note in the file, whose path is relative to the working folder. The note is
// the next role's work, so it can't be blank.
func readNote(dir, path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("want --note <file>: the note for the next role")
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	note, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read the note: %w", err)
	}

	if len(bytes.TrimSpace(note)) == 0 {
		return nil, fmt.Errorf("note %s is blank: it's the next role's work", path)
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

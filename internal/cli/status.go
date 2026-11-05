package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"uuid"

	"github.com/codedogapp/baton/internal/credits"
	"github.com/codedogapp/baton/internal/state"
)

// status lists the repo's features, each with its holder, its round, its status, and the AI
// credits that its roles have spent, in all and each (decision 66).
func status(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "status"), args); err != nil {
		return err
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	features, err := store.List()
	if err != nil {
		return err
	}

	if len(features) == 0 {
		_, err := fmt.Fprintln(env.Stdout, "No features yet.")

		return err
	}

	table := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table,
		"FEATURE\tHOLDER\tROUND\tSTATUS\tCREDITS\tLEAD\tENGINEER\tQA\tREVIEWER")

	for _, feature := range features {
		spent, err := spending(env, feature)
		if err != nil {
			return err
		}

		now := feature.Flow
		_, _ = fmt.Fprintf(table, "%s\t%s\t%d of %d\t%s\t%s\n", feature.Slug, now.Holder,
			now.Round, now.Cap+now.Extra, now.Status, strings.Join(spent, "\t"))
	}

	return table.Flush()
}

// spending returns the AI credits that the feature's roles have spent, as the status shows them:
// their sum, then each role's, in the team's order, with two decimals, or a hyphen for a role
// whose session has no events yet, and for the sum when no role's has.
func spending(env Env, feature state.Feature) ([]string, error) {
	cells, total, counted := []string{"-"}, 0.0, false

	for _, role := range team {
		spent, found, err := sessionCredits(env, *feature.Sessions.Of(role))
		if err != nil {
			return nil, fmt.Errorf("read the %s's AI credits in %s: %w", role, feature.Slug, err)
		}

		cells = append(cells, "-")
		if found {
			cells[len(cells)-1] = fmt.Sprintf("%.2f", spent)
			total, counted = total+spent, true
		}
	}

	if counted {
		cells[0] = fmt.Sprintf("%.2f", total)
	}

	return cells, nil
}

// sessionCredits returns the AI credits that the copilot session has spent, from its events in
// copilot's home folder, and whether it found them: a role's pane may not have started its
// session yet, which the zero ID says, or its copilot may not have logged an event yet.
func sessionCredits(env Env, session uuid.UUID) (float64, bool, error) {
	if session == uuid.Nil() {
		return 0, false, nil
	}

	home, err := copilotHome(env)
	if err != nil {
		return 0, false, err
	}

	spent, err := credits.Spent(filepath.Join(home, sessionState, session.String()))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}

	return spent, err == nil, err
}

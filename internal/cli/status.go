package cli

import (
	"context"
	"fmt"
	"text/tabwriter"
)

// status lists the repo's features, each with its holder, its round and its status.
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
	_, _ = fmt.Fprintln(table, "FEATURE\tHOLDER\tROUND\tSTATUS")

	for _, feature := range features {
		now := feature.Flow
		_, _ = fmt.Fprintf(table, "%s\t%s\t%d of %d\t%s\n",
			feature.Slug, now.Holder, now.Round, now.Cap+now.Extra, now.Status)
	}

	return table.Flush()
}

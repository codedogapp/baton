// Package gate runs a repo's gate: the command in baton's config file that checks the repo's code
// before a role hands a feature on (decision 56).
package gate

import (
	"context"
	"io"
	"os/exec"
)

// Run runs the command with sh in the folder, with nothing on its input and both its output and
// its errors on the writer, until it ends or the context does, and returns an error if it fails.
func Run(ctx context.Context, dir, command string, out io.Writer) error {
	// gosec's G204 wants constant arguments, but the gate is a command line by design: the user's
	// own, from the config file, which a role's file tools can't write (decision 56).
	//nolint:gosec // G204: the gate is the user's own command line, from the config file.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, out, out

	return cmd.Run()
}

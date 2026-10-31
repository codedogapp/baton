// Package cli runs baton's commands: it reads a command's arguments and its pane's environment,
// runs the command and reports how it went (decision 28).
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

// Env is what a command gets from the process that runs it.
type Env struct {
	Args   []string            // The arguments after baton's name: the command, then its own.
	Getenv func(string) string // Reads the process's environment.
	Dir    string              // The working folder; empty for the process's own.
	Stdout io.Writer
	Stderr io.Writer
}

// The exit codes of a command that didn't run to its end.
const (
	exitFailed = 1 // baton refused the command, or the command failed.
	exitUsage  = 2 // The command line doesn't parse.
)

// usage lists the commands.
const usage = `Usage: baton <command> [flags]

Commands:
  handover  hand the feature over to the next role, with a note

Run baton <command> -h for the command's flags.
`

// usageError is a command line that doesn't parse. The flag package has reported it.
type usageError struct{ error }

// Unwrap returns the flag package's error.
func (e usageError) Unwrap() error { return e.error }

// Run runs the command that the arguments name, and returns the process's exit code.
func Run(ctx context.Context, env Env) int {
	if len(env.Args) == 0 {
		_, _ = fmt.Fprint(env.Stderr, usage)

		return exitUsage
	}

	switch name, args := env.Args[0], env.Args[1:]; name {
	case "handover":
		return exit(env.Stderr, name, handover(ctx, env, args))
	case "help", "-h", "-help", "--help":
		_, _ = fmt.Fprint(env.Stdout, usage)

		return 0
	default:
		_, _ = fmt.Fprintf(env.Stderr, "baton: unknown command %q\n\n%s", name, usage)

		return exitUsage
	}
}

// exit reports the command's error, if any, and returns the process's exit code.
func exit(stderr io.Writer, name string, err error) int {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}

	if _, ok := errors.AsType[usageError](err); ok {
		return exitUsage
	}

	_, _ = fmt.Fprintf(stderr, "baton %s: %v\n", name, err)

	return exitFailed
}

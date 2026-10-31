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
	Stdin  io.Reader           // The pane's terminal, which baton pane reads while it waits.
	Stdout io.Writer
	Stderr io.Writer
	// Exec replaces the process with the program at the path, as syscall.Exec does, so it
	// returns only if it fails.
	Exec func(path string, args, env []string) error
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
  pane      wait for the role's first work, then become the role's copilot
  status    list the repo's features, with their holder, round and status
  wait      wait for the role's next work, then pick it up and print it

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
	case "pane":
		return exit(env.Stderr, name, pane(ctx, env, args))
	case "status":
		return exit(env.Stderr, name, status(ctx, env, args))
	case "wait":
		return exit(env.Stderr, name, wait(ctx, env, args))
	case "help", "-h", "-help", "--help":
		_, _ = fmt.Fprint(env.Stdout, usage)

		return 0
	default:
		_, _ = fmt.Fprintf(env.Stderr, "baton: unknown command %q\n\n%s", name, usage)

		return exitUsage
	}
}

// newFlags returns the command's flag set, which reports its errors and its usage on stderr.
func newFlags(env Env, name string) *flag.FlagSet {
	flags := flag.NewFlagSet("baton "+name, flag.ContinueOnError)
	flags.SetOutput(env.Stderr)

	return flags
}

// parse parses the command's flags, and refuses any argument after them.
func parse(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return usageError{err}
	}

	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	return nil
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

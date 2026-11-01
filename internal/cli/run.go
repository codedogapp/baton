// Package cli runs baton's commands: it reads a command's arguments and its pane's environment,
// runs the command and reports how it went (decision 28).
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/codedogapp/baton/internal/tmux"
)

// Env is what a command gets from the process that runs it.
type Env struct {
	Args   []string            // The arguments after baton's name: the command, then its own.
	Getenv func(string) string // Reads the process's environment.
	Dir    string              // The working folder; empty for the process's own.
	// Stdin is the pane's terminal, which baton pane reads while it waits, or the hook's input,
	// which baton hook stop reads.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Exec replaces the process with the program at the path, as syscall.Exec does, so it
	// returns only if it fails.
	Exec func(path string, args, env []string) error
	// Executable returns the path of baton's own program, which a feature's panes run, and the
	// plugin's hook in every pane's copilot, as os.Executable does.
	Executable func() (string, error)
	// Tmux is the user's tmux server, where baton makes its sessions.
	Tmux Server
}

// Server is a tmux server, as tmux.Server is: baton makes the sessions of the orchestrator and of
// the features there, and shows the orchestrator's in the user's terminal.
type Server interface {
	NewSession(ctx context.Context, session tmux.Session) error
	SessionDir(ctx context.Context, name string) (string, bool)
	SwitchClient(ctx context.Context, name string) error
	AttachCommand(name string) []string
}

// The exit codes of a command that didn't run to its end.
const (
	exitFailed = 1 // baton refused the command, or the command failed.
	exitUsage  = 2 // The command line doesn't parse.
)

// usage lists the commands.
const usage = `Usage: baton [<command>] [flags]

Commands:
  start          open the repo's orchestrator in its tmux session, as plain baton does
  answer         answer the question of a feature's role, in the orchestrator's pane
  ask            ask the orchestrator a question, in a role's pane, then wait for the answer
  feature start  start a feature from a brief: its branch, its worktree and its tmux session
  handover       hand a feature over to the next role, with a note
  hook stop      copilot's stop hook: send a pane's copilot that stops too soon back to work
  pane           become the pane's copilot: the orchestrator's, or the role's once it has work
  status         list the repo's features, with their holder, round and status
  wait           wait for the role's next work, then pick it up and print it

Run baton <command> -h for the command's flags.
`

// commandFunc runs one of baton's commands, with its arguments.
type commandFunc func(ctx context.Context, env Env, args []string) error

// commands are baton's commands of one word, by name.
var commands = map[string]commandFunc{
	"start":    open,
	"answer":   answer,
	"ask":      ask,
	"handover": handover,
	"pane":     pane,
	"status":   status,
	"wait":     wait,
}

// groups are baton's commands of two words, by their first word, then their second.
var groups = map[string]map[string]commandFunc{
	"feature": {"start": startFeature},
	"hook":    {"stop": hookStop},
}

// usageError is a command line that doesn't parse. The flag package has reported it.
type usageError struct{ error }

// Unwrap returns the flag package's error.
func (e usageError) Unwrap() error { return e.error }

// Run runs the command that the arguments name, or baton start if they name none, and returns the
// process's exit code.
func Run(ctx context.Context, env Env) int {
	name, args := "start", env.Args
	if len(env.Args) > 0 {
		name, args = env.Args[0], env.Args[1:]
	}

	if run, found := commands[name]; found {
		return exit(env.Stderr, name, run(ctx, env, args))
	}

	if group, found := groups[name]; found {
		return runGroup(ctx, env, group)
	}

	switch name {
	case "help", "-h", "-help", "--help":
		_, _ = fmt.Fprint(env.Stdout, usage)

		return 0
	default:
		_, _ = fmt.Fprintf(env.Stderr, "baton: unknown command %q\n\n%s", name, usage)

		return exitUsage
	}
}

// runGroup runs the group's command that the second argument names, with the arguments after it,
// and returns the process's exit code. The first argument names the group.
func runGroup(ctx context.Context, env Env, group map[string]commandFunc) int {
	name, args := env.Args[0], env.Args[1:]
	if len(args) > 0 {
		if run, found := group[args[0]]; found {
			return exit(env.Stderr, name+" "+args[0], run(ctx, env, args[1:]))
		}
	}

	words := strings.Join(slices.Sorted(maps.Keys(group)), " or ")
	_, _ = fmt.Fprintf(env.Stderr, "baton %s: want %s\n\n%s", name, words, usage)

	return exitUsage
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

// parseSlug parses the command's flags, which may come before or after a feature's slug, and
// returns the slug, or "" if there is none. It refuses any argument after the slug.
func parseSlug(flags *flag.FlagSet, args []string) (string, error) {
	if err := flags.Parse(args); err != nil {
		return "", usageError{err}
	}

	if flags.NArg() == 0 {
		return "", nil
	}

	slug := flags.Arg(0)
	if err := parse(flags, flags.Args()[1:]); err != nil {
		return "", err
	}

	return slug, nil
}

// parseTexts parses the command's flags, then returns its arguments: one for each of the names,
// which say what it is, and none of them blank. The roles quote each one for the shell.
func parseTexts(flags *flag.FlagSet, args []string, names ...string) ([]string, error) {
	if err := flags.Parse(args); err != nil {
		return nil, usageError{err}
	}

	texts := flags.Args()
	if len(texts) > len(names) {
		return nil, fmt.Errorf("unexpected argument %q: quote each text for the shell",
			texts[len(names)])
	}

	for i, name := range names {
		if i == len(texts) || strings.TrimSpace(texts[i]) == "" {
			return nil, fmt.Errorf("want %s", name)
		}
	}

	return texts, nil
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

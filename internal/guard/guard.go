// Package guard finds the commands of a bash script that the guard watches: those that act
// outside the Mac, drive the user's panes or apps, start another agent, run baton, or run a
// program that only the run decides (decision 51).
package guard

import (
	"strings"

	"github.com/codedogapp/baton/internal/bash"
)

// Kind is what a watched command does.
type Kind int

// The kinds of watched commands.
const (
	// Outward acts outside the Mac: it pushes, writes through a host's API, or logs in to a host.
	Outward Kind = iota + 1
	// Control drives the user's tmux panes or the Mac's apps, and so can type into any pane.
	Control
	// Agent starts another agent, which none of the role's limits bind.
	Agent
	// Baton runs baton, whose commands differ by pane.
	Baton
	// Unknown runs a program that only the run decides, such as "$GIT" push or eval "$X".
	Unknown
)

// Command is a watched command.
type Command struct {
	Kind Kind
	// Name is the words that make the command watched, such as "git push", with … for each part
	// that only the run decides.
	Name string
}

// always are the programs whose every use the guard watches, by kind.
var always = map[string]Kind{
	"acli": Outward, "jira": Outward, "scp": Outward, "sftp": Outward, "ssh": Outward,
	"osascript": Control, "tmux": Control,
	"aider": Agent, "claude": Agent, "codex": Agent, "copilot": Agent, "cursor-agent": Agent,
	"gemini": Agent, "opencode": Agent,
}

// Commands returns the watched commands that the script runs, in the order of bash.Calls, or an
// error if the script doesn't parse.
func Commands(script string) ([]Command, error) {
	calls, err := bash.Calls(script)
	if err != nil {
		return nil, err
	}

	var commands []Command

	for _, words := range calls {
		if command, watched := classify(bash.Name(words[0]), words[1:]); watched {
			commands = append(commands, command)
		}
	}

	return commands, nil
}

// classify returns the command that a program makes of its arguments, and whether the guard
// watches it. A program such as git-push names git's command itself.
func classify(program string, args []string) (Command, bool) {
	kind, watched := always[program]
	client, http := clients[program]

	switch {
	case watched:
		return Command{kind, program}, true
	case http:
		return Command{Outward, program}, writes(client, args)
	case program == "git":
		return git(args)
	case strings.HasPrefix(program, "git-"):
		return git(append([]string{strings.TrimPrefix(program, "git-")}, args...))
	case program == "gh":
		return gh(args)
	case program == "baton":
		return Command{Baton, name("baton", args[:min(1, len(args))]...)}, true
	case strings.Contains(program, bash.Dynamic):
		return Command{Unknown, name(program, args[:min(1, len(args))]...)}, true
	default:
		return Command{}, false
	}
}

// name returns a command's name: its words, with … for each part that only the run decides.
func name(program string, args ...string) string {
	words := append([]string{program}, args...)

	return strings.ReplaceAll(strings.Join(words, " "), bash.Dynamic, "…")
}

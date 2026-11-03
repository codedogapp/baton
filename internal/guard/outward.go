package guard

import (
	"slices"
	"strings"

	"github.com/codedogapp/baton/internal/bash"
)

// gitValued are git's options that take the next word as their value, unless they hold it after
// =, as --git-dir=.git does. Git 2.56 has dropped --super-prefix, which older gits take.
var gitValued = []string{
	"-C", "-c", "--attr-source", "--config-env", "--git-dir", "--namespace", "--shallow-file",
	"--super-prefix", "--work-tree",
}

// gitOutward are git's commands that send to another host.
var gitOutward = []string{"http-push", "imap-send", "push", "send-email", "send-pack"}

// The gh commands that only read: by their first word, such as gh status, or by their second, such
// as gh pr view.
var (
	ghReads   = []string{"--help", "--version", "-h", "help", "search", "status", "version"}
	ghReading = []string{
		"--help", "-h", "checks", "checkout", "clone", "diff", "download", "list", "status", "view",
		"watch",
	}
)

// git returns git's command in its arguments, past git's own options, and whether it acts outside
// the Mac: it is one of gitOutward, or only the run decides it. An option that only the run decides
// may take the next word, so it stands for the command.
func git(args []string) (Command, bool) {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && !strings.Contains(args[0], bash.Dynamic) {
		skip := 1
		if slices.Contains(gitValued, args[0]) {
			skip = 2
		}

		args = args[min(skip, len(args)):]
	}

	if len(args) == 0 {
		return Command{}, false
	}

	command := args[0]

	return Command{Outward, name("git", command)},
		slices.Contains(gitOutward, command) || strings.Contains(command, bash.Dynamic)
}

// gh returns gh's command in its arguments, named by its first two words, and whether it may act
// outside the Mac: any command but those that only read, or one that only the run decides, and
// gh api if it writes. GraphQL posts its reads too, so gh api graphql counts as one that writes.
func gh(args []string) (Command, bool) {
	switch {
	case len(args) == 0 || slices.Contains(ghReads, args[0]):
		return Command{}, false
	case args[0] == "api":
		return Command{Outward, "gh api"},
			slices.Contains(args, "graphql") || writes(ghAPI, args[1:])
	default:
		return Command{Outward, name("gh", args[:min(2, len(args))]...)},
			len(args) < 2 || !slices.Contains(ghReading, args[1]) ||
				strings.Contains(args[0], bash.Dynamic)
	}
}

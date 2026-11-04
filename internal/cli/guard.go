package cli

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/codedogapp/baton/internal/config"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/guard"
	"github.com/codedogapp/baton/internal/state"
)

// stopped starts each reason that baton hook guard gives for a tool that it denies, which copilot
// shows the role's agent (decision 52).
const stopped = "baton's guard stopped this call, and none of it ran. "

// why says why the guard denies a role a command of each kind, and what the role can do instead.
var why = map[guard.Kind]string{
	guard.Outward: "can act outside the Mac, which only the orchestrator does, with the user's " +
		"approval. Keep your work local, and say in your handover note what should go out.",
	guard.Control: "drives the user's tmux panes or the Mac's apps, which a role never does.",
	guard.Agent:   "starts another agent, which a role never does: do the work yourself.",
	guard.Baton: "isn't a role's command: a role runs `baton wait`, `baton handover`, " +
		"`baton ask` and `baton status`.",
	guard.Unknown: "runs a program that only the run decides, which the guard can't check: name " +
		"the program in the script itself.",
}

// helpCommands are baton's help, by the names that the guard gives them, which every pane runs.
var helpCommands = []string{"baton help", "baton -h", "baton -help", "baton --help"}

// roleCommands are baton's commands that a role runs, by the names that the guard gives them.
var roleCommands = slices.Concat(
	[]string{"baton ask", "baton handover", "baton status", "baton wait"}, helpCommands)

// guardInput is what baton reads of the preToolUse hook's input: the copilot's session and
// folder, and the tool that the copilot calls, with its arguments (facts 4 and 36).
type guardInput struct {
	Session string         `json:"sessionId"`
	Dir     string         `json:"cwd"`
	Tool    string         `json:"toolName"`
	Args    jsontext.Value `json:"toolArgs"`
}

// guardOutput is the preToolUse hook's answer, deny or ask, with the reason. Its zero value says
// nothing, which leaves the tool to copilot's own permissions.
type guardOutput struct {
	Decision string `json:"permissionDecision"`
	Reason   string `json:"permissionDecisionReason"`
}

// deny returns the answer that denies the tool for the reason, or none for no reason.
func deny(reason string) guardOutput {
	if reason == "" {
		return guardOutput{}
	}

	return guardOutput{"deny", reason}
}

// paneRules are the guard's rules in one kind of pane: its answer to the commands of a bash
// script that the guard watches, and its reason to deny a file tool's write of the path, or "".
type paneRules struct {
	script func(commands []guard.Command) (guardOutput, error)
	write  func(path string) (string, error)
}

// hookGuard is the plugin's preToolUse hook, which runs before each tool that a pane's copilot
// calls, and answers by the pane's rules: a role's (decisions 52 and 53) or the orchestrator's
// (decision 54). Copilot denies the tool when the hook fails, so baton then reports why and exits
// with 1.
func hookGuard(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "hook guard"), args); err != nil {
		return err
	}

	var input guardInput
	if err := readInput(env, &input); err != nil {
		return err
	}

	role, _, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	dir := cmp.Or(input.Dir, env.Dir)

	rules := roleRules(ctx, dir)
	if role == flow.RoleOrchestrator {
		rules = orchestratorRules(ctx, env, dir, input.Session)
	}

	answer, err := guardAnswer(dir, input, rules)
	if err != nil || answer == (guardOutput{}) {
		return err
	}

	return json.MarshalEncode(jsontext.NewEncoder(env.Stdout), answer)
}

// guardAnswer returns the guard's answer to the tool by the pane's rules: to bash by the commands
// of its script that the guard watches, and to create and edit by the file that they write. The
// pane's other tools read, or look after its shells (fact 36). It denies a script that it can't
// read in every pane. A relative path starts in the folder.
func guardAnswer(dir string, input guardInput, rules paneRules) (guardOutput, error) {
	switch input.Tool {
	case "bash":
		var args struct {
			Command string `json:"command"`
		}

		if err := json.Unmarshal(input.Args, &args); err != nil {
			return guardOutput{}, fmt.Errorf("read the arguments of bash: %w", err)
		}

		commands, err := guard.Commands(args.Command)
		if err != nil {
			return deny(fmt.Sprintf("%sThe guard can't read the script (%v): write it in "+
				"plain bash.", stopped, err)), nil
		}

		return rules.script(commands)
	case "create", "edit":
		var args struct {
			Path string `json:"path"`
		}

		if err := json.Unmarshal(input.Args, &args); err != nil {
			return guardOutput{}, fmt.Errorf("read the arguments of %s: %w", input.Tool, err)
		}

		reason, err := rules.write(absolute(dir, args.Path))

		return deny(reason), err
	default:
		return guardOutput{}, nil
	}
}

// firstAnswer returns the judge's answer to the first command that it denies, or else to the
// first that it asks the user about, or none.
func firstAnswer(commands []guard.Command, judge func(guard.Command) guardOutput) guardOutput {
	var asked guardOutput

	for _, command := range commands {
		answer := judge(command)
		if answer.Decision == "deny" {
			return answer
		}

		asked = cmp.Or(asked, answer)
	}

	return asked
}

// absolute returns the path, made absolute from the folder if it is relative. An empty folder is
// the process's own working folder, as in main.
func absolute(dir, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	return path
}

// roleRules are the guard's rules in a role's pane, which nobody watches: it denies each command
// that it watches but baton's commands for a role, and a write in baton's folder or its cache
// folder (decisions 52 and 53).
func roleRules(ctx context.Context, dir string) paneRules {
	return paneRules{
		script: func(commands []guard.Command) (guardOutput, error) {
			return firstAnswer(commands, roleCommand), nil
		},
		write: func(path string) (string, error) { return pathReason(ctx, dir, path) },
	}
}

// roleCommand returns the guard's answer to a command that it watches in a role's pane.
func roleCommand(command guard.Command) guardOutput {
	if slices.Contains(roleCommands, command.Name) {
		return guardOutput{}
	}

	return deny(fmt.Sprintf("%s`%s` %s", stopped, command.Name, why[command.Kind]))
}

// pathReason returns the reason to deny a file tool's write of the absolute path in a role's pane,
// or "" to let it run: a file in baton's folder, which only baton writes (decision 48), in baton's
// cache folder, whose plugin runs this guard (decision 53), or in baton's config folder, whose file
// holds the repo's gate (decision 56).
func pathReason(ctx context.Context, dir, path string) (string, error) {
	cache, err := batonCache()

	var configDir string
	if err == nil {
		configDir, err = userConfigDir()
	}

	var store state.Store
	if err == nil {
		store, err = openStore(ctx, dir)
	}

	if err != nil {
		return "", err
	}

	switch {
	case within(path, store.Dir):
		return fmt.Sprintf("%s%s is in baton's folder %s, which only baton writes: write your "+
			"notes in your session's files/ folder.", stopped, path, store.Dir), nil
	case within(path, cache):
		return fmt.Sprintf("%s%s is in baton's cache folder %s, which only baton writes: it "+
			"holds the plugin whose hook runs this guard.", stopped, path, cache), nil
	case within(path, config.Folder(configDir)):
		return fmt.Sprintf("%s%s is in baton's config folder %s, which only the user writes: "+
			"its file holds the gate that your handovers run.", stopped, path,
			config.Folder(configDir)), nil
	}

	return "", nil
}

// within reports whether the path is the folder or lies in it, by the files that they name, so
// that neither a symlink nor a disk that ignores case hides it. A path that isn't there yet, such
// as a new file's, counts by the part of it that is. A folder that isn't there holds nothing.
func within(path, folder string) bool {
	want, err := os.Stat(folder)
	if err != nil {
		return false
	}

	for dir := resolve(filepath.Clean(path)); ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(dir); err == nil && os.SameFile(info, want) {
			return true
		}

		if dir == filepath.Dir(dir) {
			return false
		}
	}
}

// resolve returns the path with the symlinks resolved in the longest part of it that is there,
// and the rest as it is.
func resolve(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil || path == filepath.Dir(path) {
		return cmp.Or(resolved, path)
	}

	return filepath.Join(resolve(filepath.Dir(path)), filepath.Base(path))
}

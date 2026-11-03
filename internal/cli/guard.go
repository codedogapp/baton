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

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/guard"
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

// roleCommands are baton's commands that a role runs, by the names that the guard gives them.
var roleCommands = []string{
	"baton ask", "baton handover", "baton status", "baton wait",
	"baton help", "baton -h", "baton -help", "baton --help",
}

// guardInput is what baton reads of the preToolUse hook's input: the copilot's folder, and the
// tool that the copilot calls, with its arguments (facts 4 and 36).
type guardInput struct {
	Dir  string         `json:"cwd"`
	Tool string         `json:"toolName"`
	Args jsontext.Value `json:"toolArgs"`
}

// guardOutput is the preToolUse hook's answer that denies the tool, with the reason.
type guardOutput struct {
	Decision string `json:"permissionDecision"`
	Reason   string `json:"permissionDecisionReason"`
}

// hookGuard is the plugin's preToolUse hook, which runs before each tool that a pane's copilot
// calls. In a role's pane, it denies a bash script that the guard can't read, or that runs a
// command that the guard watches other than baton's commands for a role, and a create or edit of
// a file in baton's folder or in baton's cache folder, which holds the plugin. It says nothing in
// the orchestrator's pane yet. Copilot denies the tool when the hook fails, so baton then reports
// why and exits with 1 (decisions 52 and 53).
func hookGuard(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "hook guard"), args); err != nil {
		return err
	}

	var input guardInput
	if err := readInput(env, &input); err != nil {
		return err
	}

	role, _, err := fromPane(env.Getenv)
	if err != nil || role == flow.RoleOrchestrator {
		return err
	}

	reason, err := guardReason(ctx, cmp.Or(input.Dir, env.Dir), input)
	if err != nil || reason == "" {
		return err
	}

	return json.MarshalEncode(jsontext.NewEncoder(env.Stdout), guardOutput{"deny", reason})
}

// guardReason returns the reason to deny the tool in a role's pane, or "" to let it run: bash by
// its script, and create and edit by the file that they write. The role's other tools read, or
// look after its shells (fact 36). A relative path starts in the folder.
func guardReason(ctx context.Context, dir string, input guardInput) (string, error) {
	switch input.Tool {
	case "bash":
		var args struct {
			Command string `json:"command"`
		}

		if err := json.Unmarshal(input.Args, &args); err != nil {
			return "", fmt.Errorf("read the arguments of bash: %w", err)
		}

		return scriptReason(args.Command), nil
	case "create", "edit":
		var args struct {
			Path string `json:"path"`
		}

		if err := json.Unmarshal(input.Args, &args); err != nil {
			return "", fmt.Errorf("read the arguments of %s: %w", input.Tool, err)
		}

		return pathReason(ctx, dir, args.Path)
	default:
		return "", nil
	}
}

// scriptReason returns the reason to deny a bash script in a role's pane, or "" to let it run: a
// script that the guard can't read, or its first command that the guard watches other than
// baton's commands for a role.
func scriptReason(script string) string {
	commands, err := guard.Commands(script)
	if err != nil {
		return fmt.Sprintf("%sThe guard can't read the script (%v): write it in plain bash.",
			stopped, err)
	}

	for _, command := range commands {
		if !slices.Contains(roleCommands, command.Name) {
			return fmt.Sprintf("%s`%s` %s", stopped, command.Name, why[command.Kind])
		}
	}

	return ""
}

// pathReason returns the reason to deny a file tool's write in a role's pane, or "" to let it run:
// a file in baton's folder, which only baton writes (decision 48), or in baton's cache folder,
// whose plugin runs this guard (decision 53).
func pathReason(ctx context.Context, dir, path string) (string, error) {
	cache, err := batonCache()
	if err != nil {
		return "", err
	}

	store, err := openStore(ctx, dir)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	// An empty dir is the process's own working folder, as in main.
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	switch {
	case within(path, store.Dir):
		return fmt.Sprintf("%s%s is in baton's folder %s, which only baton writes: write your "+
			"notes in your session's files/ folder.", stopped, path, store.Dir), nil
	case within(path, cache):
		return fmt.Sprintf("%s%s is in baton's cache folder %s, which only baton writes: it "+
			"holds the plugin whose hook runs this guard.", stopped, path, cache), nil
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

package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/plugin"
)

// cacheFolder is baton's folder in the user's cache folder, which holds the plugin (decision 36).
const cacheFolder = "baton"

// model is every role's model until the config file names one (decision 7).
const model = "claude-opus-5.5"

// allowAll is copilot's variable that allows every tool and, set to "true" exactly, also trusts
// the folder that copilot starts in, the feature's worktree, for that session (decision 39).
const allowAll = "COPILOT_ALLOW_ALL"

// pane becomes the pane's copilot. The orchestrator's copilot starts at once, since the user talks
// to it. A role's pane waits for the role's first work, then becomes the role's copilot, with that
// work as its first prompt (decision 38). It finds copilot and writes the plugin before it waits,
// so that a pane that can't start copilot never picks up work that nobody would then do.
func pane(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "pane"), args); err != nil {
		return err
	}

	role, slug, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	program, dir, err := prepare()
	if err != nil {
		return err
	}

	if role == flow.RoleOrchestrator {
		return becomeCopilot(env, program, orchestrating(dir), os.Environ())
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(env.Stdout, "%s in %s: waiting for work.\n", role, slug); err != nil {
		return err
	}

	waiting, stop := context.WithCancelCause(ctx)
	defer stop(nil)

	go watch(env.Stdin, stop)

	feature, err := pickUp(waiting, store, role, slug)
	if err != nil {
		return err
	}

	work := pickupText(store, feature, role)

	return becomeCopilot(env, program, command(role, dir, work), environment())
}

// becomeCopilot replaces baton with copilot, with the command line and the environment.
func becomeCopilot(env Env, program string, line, vars []string) error {
	if err := env.Exec(program, line, vars); err != nil {
		return fmt.Errorf("start copilot: %w", err)
	}

	return nil
}

// prepare finds copilot and writes the plugin into baton's cache folder, and returns copilot's
// path and the plugin's folder.
func prepare() (program, dir string, err error) {
	program, err = exec.LookPath("copilot")
	if err != nil {
		return "", "", err
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("find the cache folder: %w", err)
	}

	dir, err = plugin.Write(filepath.Join(cache, cacheFolder))
	if err != nil {
		return "", "", err
	}

	return program, dir, nil
}

// watch reads the pane's terminal, and drops what the user types there, until the terminal's
// input ends, then stops the wait. Once a terminal is gone, a read of it ends, even in a program
// that missed the hangup (facts 22 and 25). Ctrl-D ends the input too.
func watch(terminal io.Reader, stop context.CancelCauseFunc) {
	_, err := io.Copy(io.Discard, terminal)
	stop(fmt.Errorf("the terminal's input ended: %w", cmp.Or(err, io.EOF)))
}

// environment returns baton's environment for copilot, with allowAll set to "true" in place of the
// user's own value, since copilot reads the first of two (fact 9).
func environment() []string {
	vars := slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return strings.HasPrefix(variable, allowAll+"=")
	})

	return append(vars, allowAll+"=true")
}

// tools are the tools that every pane's copilot has: none that runs agents the user can't see,
// and none of the user's skills or MCP tools (decision 10).
var tools = []string{
	"bash", "read_bash", "stop_bash", "list_bash", "view", "create", "edit", "grep", "glob",
	"web_fetch",
}

// command returns the role's copilot command line: the role's agent in the plugin, the model,
// every permission and no questions, the tools, and the role's work as the first prompt
// (decisions 9 to 11).
func command(role flow.Role, dir, work string) []string {
	return slices.Concat([]string{
		"copilot", "--agent", plugin.Agent(role), "--plugin-dir", dir, "--model", model,
		"--allow-all", "--no-ask-user", "--available-tools",
	}, tools, []string{"-i", work})
}

// orchestrating returns the orchestrator's copilot command line: its agent in the plugin, the
// model, and the tools, with ask_user and the user's nudge tools, since the user talks to it. It
// keeps copilot's own prompts for the tools, and has no first prompt, since the user speaks first
// (decision 45).
func orchestrating(dir string) []string {
	return slices.Concat([]string{
		"copilot", "--agent", plugin.Agent(flow.RoleOrchestrator), "--plugin-dir", dir,
		"--model", model, "--available-tools",
	}, tools, []string{"ask_user", "nudge-ask", "nudge-report"})
}

package cli

import (
	"cmp"
	"context"
	"errors"
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

// pane waits for the first work of the pane's role, then becomes the role's copilot, with that
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

	if role == flow.RoleOrchestrator {
		return errors.New("the orchestrator has no pane: baton pane runs a feature's roles")
	}

	program, dir, err := prepare()
	if err != nil {
		return err
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

	err = env.Exec(program, command(role, dir, pickupText(store, feature, role)), environment())
	if err != nil {
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

// command returns the role's copilot command line: the role's agent in the plugin, the model,
// every permission and no questions, the tools, and the role's work as the first prompt
// (decisions 9 to 11).
func command(role flow.Role, dir, work string) []string {
	return []string{
		"copilot",
		"--agent", plugin.Agent(role),
		"--plugin-dir", dir,
		"--model", model,
		"--allow-all", "--no-ask-user",
		"--available-tools", "bash", "read_bash", "stop_bash", "list_bash",
		"view", "create", "edit", "grep", "glob", "web_fetch",
		"-i", work,
	}
}

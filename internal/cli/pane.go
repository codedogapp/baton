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
	"strconv"
	"strings"

	"github.com/codedogapp/baton/internal/config"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/plugin"
)

// cacheFolder is baton's folder in the user's cache folder, which holds the plugin (decision 36).
const cacheFolder = "baton"

// allowAll is copilot's variable that allows every tool and, set to "true" exactly, also trusts
// the folder that copilot starts in, the feature's worktree, for that session (decision 39).
const allowAll = "COPILOT_ALLOW_ALL"

// envBin is the variable that names baton's own program in every pane's copilot, for the
// plugin's hooks, which run baton hook guard and baton hook stop (decisions 47 and 52).
const envBin = "BATON_BIN"

// pane becomes the pane's copilot, with the role's settings from the config file (decision 49).
// The orchestrator's copilot starts at once, since the user talks to it. A role's pane waits for
// the role's first work, then becomes the role's copilot, with that work as its first prompt
// (decision 38). It finds copilot, writes the plugin and reads the settings before it waits, so
// that a pane that can't start copilot never picks up work that nobody would then do. After the
// wait it checks the plugin again, which a role may have changed meanwhile (decision 53).
func pane(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "pane"), args); err != nil {
		return err
	}

	role, slug, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	self, err := env.Executable()
	if err != nil {
		return fmt.Errorf("find baton's own program: %w", err)
	}

	program, dir, err := prepare()
	if err != nil {
		return err
	}

	settings, err := loadSettings(ctx, env.Dir)
	if err != nil {
		return err
	}

	own, bin := settings.Roles[role], envBin+"="+self
	if role == flow.RoleOrchestrator {
		return becomeCopilot(env, program, orchestrating(dir, own), environment(bin))
	}

	work, err := waitForWork(ctx, env, role, slug)
	if err != nil {
		return err
	}

	if dir, err = writePlugin(); err != nil {
		return err
	}

	return becomeCopilot(env, program, command(role, dir, own, work),
		environment(bin, allowAll+"=true"))
}

// waitForWork says that the role waits, waits for the role's work in the feature, then picks it
// up and returns it, as the role's first prompt. While it waits, it marks the role as waiting, as
// baton wait does (decision 47), and drops what the user types in the pane; it stops waiting when
// the pane's terminal ends.
func waitForWork(ctx context.Context, env Env, role flow.Role, slug string) (string, error) {
	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return "", err
	}

	if _, err := fmt.Fprintf(env.Stdout, "%s in %s: waiting for work.\n", role, slug); err != nil {
		return "", err
	}

	waiting, stop := context.WithCancelCause(ctx)
	defer stop(nil)

	go watch(env.Stdin, stop)

	feature, err := pickUp(waiting, store, role, slug)
	if err != nil {
		return "", err
	}

	return pickupText(store, feature, role), nil
}

// becomeCopilot replaces baton with copilot, with the command line and the environment.
func becomeCopilot(env Env, program string, line, vars []string) error {
	if err := env.Exec(program, line, vars); err != nil {
		return fmt.Errorf("start copilot: %w", err)
	}

	return nil
}

// prepare finds copilot and writes the plugin, and returns copilot's path and the plugin's folder.
func prepare() (program, dir string, err error) {
	program, err = exec.LookPath("copilot")
	if err != nil {
		return "", "", err
	}

	dir, err = writePlugin()
	if err != nil {
		return "", "", err
	}

	return program, dir, nil
}

// writePlugin writes the plugin into baton's cache folder, unless a folder there holds it
// already, unchanged, and returns the plugin's folder (decisions 36 and 53).
func writePlugin() (string, error) {
	cache, err := batonCache()
	if err != nil {
		return "", err
	}

	return plugin.Write(cache)
}

// batonCache returns baton's folder in the user's cache folder, which holds the plugin (decision
// 36).
func batonCache() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find the cache folder: %w", err)
	}

	return filepath.Join(cache, cacheFolder), nil
}

// watch reads the pane's terminal, and drops what the user types there, until the terminal's
// input ends, then stops the wait. Once a terminal is gone, a read of it ends, even in a program
// that missed the hangup (facts 22 and 25). Ctrl-D ends the input too.
func watch(terminal io.Reader, stop context.CancelCauseFunc) {
	_, err := io.Copy(io.Discard, terminal)
	stop(fmt.Errorf("the terminal's input ended: %w", cmp.Or(err, io.EOF)))
}

// environment returns baton's environment for copilot, with the variables, each one name=value,
// in place of the user's own values for their names, since copilot reads the first of two (fact
// 9).
func environment(vars ...string) []string {
	names := make([]string, 0, len(vars))
	for _, variable := range vars {
		name, _, _ := strings.Cut(variable, "=")
		names = append(names, name)
	}

	kept := slices.DeleteFunc(os.Environ(), func(variable string) bool {
		name, _, _ := strings.Cut(variable, "=")

		return slices.Contains(names, name)
	})

	return append(kept, vars...)
}

// tools are the tools that every pane's copilot has: none that runs agents the user can't see,
// and none of the user's skills or MCP tools (decision 10).
var tools = []string{
	"bash", "read_bash", "stop_bash", "list_bash", "view", "create", "edit", "grep", "glob",
	"web_fetch",
}

// command returns the role's copilot command line: the role's agent in the plugin, the role's
// settings, every permission and no questions, the tools, and the role's work as the first prompt
// (decisions 9 to 11 and 49).
func command(role flow.Role, dir string, own config.Role, work string) []string {
	return slices.Concat([]string{"copilot", "--agent", plugin.Agent(role), "--plugin-dir", dir},
		settingFlags(own), []string{"--allow-all", "--no-ask-user", "--available-tools"}, tools,
		[]string{"-i", work})
}

// orchestrating returns the orchestrator's copilot command line: its agent in the plugin, its
// settings, every permission, since baton's guard asks the user what needs asking, and the tools,
// with ask_user and its extra tools, the user's own, such as nudge's, since the user talks to it.
// It has no first prompt, since the user speaks first (decisions 45, 49 and 54).
func orchestrating(dir string, own config.Role) []string {
	return slices.Concat([]string{
		"copilot", "--agent", plugin.Agent(flow.RoleOrchestrator), "--plugin-dir", dir,
	}, settingFlags(own), []string{"--allow-all", "--available-tools"}, tools,
		[]string{"ask_user"}, own.ExtraTools)
}

// settingFlags returns copilot's flags for the role's settings: its model, and its reasoning
// effort and its cap on AI credits, unless it leaves them to copilot (fact 31). The cap is in
// digits, never in an exponent's form.
func settingFlags(own config.Role) []string {
	flags := []string{"--model", own.Model}

	if own.ReasoningEffort != "" {
		flags = append(flags, "--reasoning-effort", own.ReasoningEffort)
	}

	if own.MaxAICredits > 0 {
		flags = append(flags, "--max-ai-credits",
			strconv.FormatFloat(own.MaxAICredits, 'f', -1, 64))
	}

	return flags
}

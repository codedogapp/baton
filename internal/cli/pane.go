package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"uuid"

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

// envPath is the variable that lists the folders where a shell looks for a program by its name.
const envPath = "PATH"

// pane becomes the pane's copilot, with the role's settings from the config file (decision 49),
// and with the folder of baton's own program first on its PATH (decision 62). The orchestrator's
// copilot starts at once, since the user talks to it, in the session that baton keeps for it, once
// the pane has given back what its last copilot's waits picked up (decision 68). A role's pane
// waits for the role's first work, then becomes the role's copilot, with that work as its first
// prompt (decision 38), in the role's session in the feature, whose ID baton keeps (decision 66),
// and with the belt in its environment (decision 55); a pane that starts again while its role
// holds the work that it picked up carries on with that work at once (decision 67). It finds
// copilot, writes the plugin, reads the settings and makes the belt before it waits, so that a pane
// that can't start copilot never picks up work that nobody would then do. After the wait it checks
// the plugin again, which a role may have changed meanwhile (decision 53).
func pane(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "pane"), args); err != nil {
		return err
	}

	role, slug, err := fromPane(env.Getenv)
	if err != nil {
		return err
	}

	start, err := prepare(ctx, env, role)
	if err != nil {
		return err
	}

	if role == flow.RoleOrchestrator {
		return becomeOrchestrator(ctx, env, start)
	}

	vars, err := belted(ctx, env.Dir, append(start.vars, allowAll+"=true")...)
	if err != nil {
		return err
	}

	turn, err := firstWork(ctx, env, role, slug)
	if err != nil {
		return err
	}

	dir, err := writePlugin()
	if err != nil {
		return err
	}

	return becomeCopilot(env, start.program, command(role, dir, start.own, turn), vars)
}

// becomeCopilot replaces baton with copilot, with the command line and the environment.
func becomeCopilot(env Env, program string, line, vars []string) error {
	if err := env.Exec(program, line, vars); err != nil {
		return fmt.Errorf("start copilot: %w", err)
	}

	return nil
}

// copilotStart is what the pane starts copilot with: copilot's path, the plugin's folder, the
// role's settings, and baton's variables for copilot, each one name=value.
type copilotStart struct {
	program, plugin string
	own             config.Role
	vars            []string
}

// prepare finds baton's own program and copilot, writes the plugin and reads the role's settings,
// for the pane's copilot, whose PATH finds baton's own program first (decision 62).
func prepare(ctx context.Context, env Env, role flow.Role) (copilotStart, error) {
	self, err := env.Executable()
	if err != nil {
		return copilotStart{}, fmt.Errorf("find baton's own program: %w", err)
	}

	program, err := exec.LookPath("copilot")
	if err != nil {
		return copilotStart{}, err
	}

	dir, err := writePlugin()
	if err != nil {
		return copilotStart{}, err
	}

	settings, err := loadSettings(ctx, env.Dir)
	if err != nil {
		return copilotStart{}, err
	}

	return copilotStart{
		program: program, plugin: dir, own: settings.Roles[role],
		vars: []string{envBin + "=" + self, envPath + "=" + findable(self)},
	}, nil
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

// findable returns the PATH for the pane's copilot, so that the agents' baton commands run the
// pane's baton, as its hooks do: the pane's own PATH, which found copilot, so it isn't empty, with
// the folder of baton's own program first, unless the PATH finds that program as baton already, or
// the folder's name holds the separator of a PATH's folders, which no PATH can name (decision 62).
func findable(self string) string {
	path, dir := os.Getenv(envPath), filepath.Dir(self)
	if strings.ContainsRune(dir, filepath.ListSeparator) || finds(self) {
		return path
	}

	return dir + string(filepath.ListSeparator) + path
}

// finds reports whether the PATH finds baton's own program as baton, itself or through a link.
func finds(self string) bool {
	want, err := os.Stat(self)
	if err != nil {
		return false
	}

	found, err := exec.LookPath("baton")
	if err != nil {
		return false
	}

	got, err := os.Stat(found)

	return err == nil && os.SameFile(got, want)
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
// settings, the turn's session, every permission and no questions, the tools, and the role's work
// as the first prompt (decisions 9 to 11, 49 and 66).
func command(role flow.Role, dir string, own config.Role, turn firstTurn) []string {
	return slices.Concat([]string{"copilot", "--agent", plugin.Agent(role), "--plugin-dir", dir},
		settingFlags(own), []string{
			"--session-id", turn.session.String(), "--allow-all", "--no-ask-user",
			"--available-tools",
		}, tools, []string{"-i", turn.work})
}

// orchestrating returns the orchestrator's copilot command line: its agent in the plugin, its
// settings, its session, every permission, since baton's guard asks the user what needs asking,
// and the tools, with ask_user and its extra tools, the user's own, such as nudge's, since the user
// talks to it. It has no first prompt, since the user speaks first (decisions 45, 49, 54 and 68).
func orchestrating(dir string, own config.Role, session uuid.UUID) []string {
	return slices.Concat([]string{
		"copilot", "--agent", plugin.Agent(flow.RoleOrchestrator), "--plugin-dir", dir,
	}, settingFlags(own), []string{
		"--session-id", session.String(), "--allow-all", "--available-tools",
	}, tools, []string{"ask_user"}, own.ExtraTools)
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

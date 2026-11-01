package cli_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The pane tests put a stand-in copilot first on the PATH, and the user's cache folder in a home
// folder of the test's own. No test runs copilot: baton pane gives Env.Exec the command line that
// would replace baton, and the tests' Exec records it.

// TestPaneBecomesTheRolesCopilot has the lead's pane pick up the orchestrator's brief, write the
// plugin, and become the lead's copilot, with the brief as its first prompt, and with
// COPILOT_ALLOW_ALL=true in place of the user's own value, which trusts no folder (fact 9), and
// BATON_BIN naming this baton in place of the user's (decision 47). A variable whose name only
// starts with one of theirs stays.
func TestPaneBecomesTheRolesCopilot(t *testing.T) {
	copilot, cache := standIn(t)
	t.Setenv("COPILOT_ALLOW_ALL", "1")
	t.Setenv("BATON_BIN", "/usr/local/bin/baton")
	t.Setenv("BATON_BIN_OLD", "/opt/baton/bin/baton")

	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		env, _ := inPane(t, linked, paneOf(flow.RoleLead))

		wantCopilot(t, paneFor(t, env), flow.RoleLead, execed{
			path: copilot,
			args: copilotLine(flow.RoleLead, pluginIn(t, cache),
				"Picked up demo-1 as lead: round 0 of 3, from orchestrator.\nNote: "+
					notePath(store, "01-orchestrator-to-lead.md")+"\n"),
			env: copilotEnv(),
		})
		wantWork(t, store, state.Work{
			Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator, Picked: true,
		})
		wantJournal(t, store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"pickup lead  ", ""},
		})
	})
}

// TestPaneWaitsForTheRolesWork has the engineer's pane wait, and drop what the user types, while
// the lead picks up its own work, then become the engineer's copilot within a second of the
// lead's handover.
func TestPaneWaitsForTheRolesWork(t *testing.T) {
	copilot, cache := standIn(t)

	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		env, user := inPane(t, linked, paneOf(flow.RoleEngineer))
		engineer := make(chan ran, 1)

		go func() { engineer <- paneFor(t, env) }()

		time.Sleep(time.Minute)

		if _, err := io.WriteString(user, "Are you there?\n"); err != nil {
			t.Fatal(err)
		}

		if got := waitAs(t, mainDir, flow.RoleLead); got.code != 0 {
			t.Fatalf("the lead's wait: %+v", got)
		}

		time.Sleep(time.Minute)
		synctest.Wait()

		select {
		case got := <-engineer:
			t.Fatalf("the engineer's pane ended before its work came: %+v", got)
		default:
		}

		handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

		handedOver := time.Now()
		got := <-engineer

		if waited := time.Since(handedOver); waited > time.Second {
			t.Errorf("the engineer's copilot started %v after the handover; want a second", waited)
		}

		wantCopilot(t, got, flow.RoleEngineer, execed{
			path: copilot,
			args: copilotLine(flow.RoleEngineer, pluginIn(t, cache),
				"Picked up demo-1 as engineer: round 1 of 3, from lead.\nNote: "+
					notePath(store, "02-lead-to-engineer.md")+"\n"),
			env: copilotEnv(),
		})
	})
}

// TestPaneBecomesTheOrchestratorsCopilot has the orchestrator's pane write the plugin and become
// the orchestrator's copilot at once, with no work to wait for and no first prompt, since the user
// speaks first, and with copilot's own prompts: the user's environment as it is, but for BATON_BIN,
// which names this baton for the stop hook (decisions 45 and 47).
func TestPaneBecomesTheOrchestratorsCopilot(t *testing.T) {
	copilot, cache := standIn(t)
	t.Setenv("COPILOT_ALLOW_ALL", "1")
	t.Setenv("BATON_BIN", "/usr/local/bin/baton")

	mainDir, _ := repos(t)
	env, _ := inPane(t, mainDir, orchestratorPane())

	got := runExec(t.Context(), env, nil)
	if got.result != (result{}) {
		t.Errorf("got %+v; want exit code 0 and no output", got.result)
	}

	wantExec(t, got.execs, execed{
		path: copilot,
		args: []string{
			"copilot", "--agent", "baton:orchestrator", "--plugin-dir", pluginIn(t, cache),
			"--model", "claude-opus-5.5", "--available-tools", "bash", "read_bash", "stop_bash",
			"list_bash", "view", "create", "edit", "grep", "glob", "web_fetch", "ask_user",
			"nudge-ask", "nudge-report",
		},
		env: append(withoutVars("BATON_BIN"), "BATON_BIN="+self),
	})
}

// standIn puts a stand-in copilot first on the PATH, and moves the home folder, and with it the
// user's cache folder, into the test's own. It returns the stand-in's path and baton's folder in
// that cache folder.
func standIn(t *testing.T) (copilot, cache string) {
	t.Helper()

	copilot = onPath(t, "copilot")

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")

	userCache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	return copilot, filepath.Join(userCache, "baton")
}

// inPane returns baton pane's environment in the pane with the variables, from the folder, and
// the user's end of the pane's terminal, which stays open until the test ends.
func inPane(t *testing.T, dir string, vars map[string]string) (cli.Env, *io.PipeWriter) {
	t.Helper()

	terminal, user := io.Pipe()

	t.Cleanup(func() { _ = user.Close() })

	return cli.Env{
		Args:       []string{"pane"},
		Getenv:     func(name string) string { return vars[name] },
		Dir:        dir,
		Stdin:      terminal,
		Executable: func() (string, error) { return self, nil },
	}, user
}

// paneFor runs baton pane like runExec, with an Exec that succeeds, in a bubble, and stops it
// after an hour of the bubble's time, so a pane that should have ended fails the test at once.
func paneFor(t *testing.T, env cli.Env) ran {
	t.Helper()

	ctx, stop := context.WithTimeout(t.Context(), time.Hour)
	defer stop()

	return runExec(ctx, env, nil)
}

// copilotLine returns the role's copilot command line, with the plugin's folder and the role's
// work as its first prompt (decisions 7, 9 to 11 and 38).
func copilotLine(role flow.Role, plugin, work string) []string {
	return []string{
		"copilot", "--agent", "baton:" + string(role), "--plugin-dir", plugin,
		"--model", "claude-opus-5.5", "--allow-all", "--no-ask-user",
		"--available-tools", "bash", "read_bash", "stop_bash", "list_bash",
		"view", "create", "edit", "grep", "glob", "web_fetch",
		"-i", work,
	}
}

// copilotEnv returns the environment that a role's copilot gets: baton's, with BATON_BIN naming
// this baton, for the stop hook, and COPILOT_ALLOW_ALL=true, which trusts copilot's folder, each in
// place of the user's own value, since copilot reads the first of two (fact 9, decisions 39 and
// 47).
func copilotEnv() []string {
	return append(withoutVars("BATON_BIN", "COPILOT_ALLOW_ALL"), "BATON_BIN="+self,
		"COPILOT_ALLOW_ALL=true")
}

// withoutVars returns baton's environment without the variables with the names.
func withoutVars(names ...string) []string {
	return slices.DeleteFunc(os.Environ(), func(variable string) bool {
		name, _, _ := strings.Cut(variable, "=")

		return slices.Contains(names, name)
	})
}

// pluginName is the name of the plugin's folder in the cache folder (decision 36).
var pluginName = regexp.MustCompile(`^plugin-[0-9a-f]{16}$`)

// pluginIn returns the plugin's folder in the cache folder, which holds nothing else: no
// temporary folder that baton pane failed to rename.
func pluginIn(t *testing.T, cache string) string {
	t.Helper()

	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 || !pluginName.MatchString(entries[0].Name()) {
		t.Fatalf("the cache folder holds %v; want the plugin's folder alone", entries)
	}

	return filepath.Join(cache, entries[0].Name())
}

// wantCopilot checks that baton pane waited as the role, then gave Env.Exec the command line, and
// no other.
func wantCopilot(t *testing.T, got ran, role flow.Role, want execed) {
	t.Helper()

	waited := result{stdout: string(role) + " in demo-1: waiting for work.\n"}
	if got.result != waited {
		t.Errorf("got %+v; want %+v", got.result, waited)
	}

	wantExec(t, got.execs, want)
}

// wantRefused checks that baton pane failed for the reason, after it printed the output, and
// that it gave Env.Exec no command line.
func wantRefused(t *testing.T, got ran, stdout, reason string) {
	t.Helper()

	want := "baton pane: " + reason
	if got.code != 1 || got.stdout != stdout || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("got %+v; want exit code 1, %q and %q", got.result, stdout, want)
	}

	if len(got.execs) != 0 {
		t.Errorf("Exec got %q; want no command line", got.execs)
	}
}

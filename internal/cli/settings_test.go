package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The settings tests write baton's config file in a home folder of the test's own, and give the
// repo an origin remote, whose URL picks the repo's section in the file (decision 49).

// TestPaneGivesTheRoleItsSettings has the lead's pane become the lead's copilot with the lead's
// settings: those for every repo, under those of the repo's section, but not another repo's. Its
// belt blocks pushes to the origin remote's URL (decision 55).
func TestPaneGivesTheRoleItsSettings(t *testing.T) {
	copilot, cache := standIn(t)
	writeConfig(t, `{
		"roles": {"lead": {"model": "gpt-5.6-sol", "reasoningEffort": "high", "maxAiCredits": 40}},
		"repos": {
			"github.com/codedogapp/demo": {"roles": {"lead": {"maxAiCredits": 1000000.5}}},
			"github.com/codedogapp/other": {"roles": {"lead": {"model": "claude-haiku-5.5"}}}
		}
	}`)

	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "remote", "add", "origin",
			"https://github.com/codedogapp/demo.git"))
		store := started(t, mainDir, fresh())

		env, _ := inPane(t, linked, paneOf(flow.RoleLead))

		wantCopilot(t, paneFor(t, env), flow.RoleLead, execed{
			path: copilot,
			args: []string{
				"copilot", "--agent", "baton:lead", "--plugin-dir", pluginIn(t, cache),
				"--model", "gpt-5.6-sol", "--reasoning-effort", "high",
				"--max-ai-credits", "1000000.5", "--session-id", sessionOf(t, store, flow.RoleLead),
				"--allow-all", "--no-ask-user",
				"--available-tools", "bash", "read_bash", "stop_bash", "list_bash", "view",
				"create", "edit", "grep", "glob", "web_fetch",
				"-i", "Picked up demo-1 as lead: round 0 of 3, from orchestrator.\nNote: " +
					notePath(store, "01-orchestrator-to-lead.md") + "\n",
			},
			env: copilotEnv(pushRule("https://github.com/codedogapp/demo.git")),
		})
	})
}

// TestPaneGivesTheOrchestratorItsSettings has the orchestrator's pane, in a repo without an origin
// remote, become the orchestrator's copilot with the settings for every repo: here no extra tools,
// so not nudge's, and copilot's own reasoning effort.
func TestPaneGivesTheOrchestratorItsSettings(t *testing.T) {
	copilot, cache := standIn(t)
	writeConfig(t, `{
		"roles": {"orchestrator": {"model": "claude-sonnet-5.5", "maxAiCredits": 30,
			"extraTools": []}},
		"repos": {"github.com/codedogapp/demo": {"roles": {"orchestrator": {"model": "auto"}}}}
	}`)

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
			"--model", "claude-sonnet-5.5", "--max-ai-credits", "30", "--allow-all",
			"--available-tools", "bash", "read_bash", "stop_bash", "list_bash", "view", "create",
			"edit", "grep", "glob", "web_fetch", "ask_user",
		},
		env: orchestratorEnv(),
	})
}

// TestStartFeatureTakesItsSettings starts a feature with the round cap for every repo, and the
// branch prefix of the repo's section.
func TestStartFeatureTakesItsSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeConfig(t, `{
		"gate": "true",
		"roundCap": 5,
		"repos": {"github.com/codedogapp/demo": {"branchPrefix": "team/"}}
	}`)

	mainDir := repoNamed(t, "main")
	gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "remote", "add", "origin",
		"git@github.com:codedogapp/demo.git"))
	brief := write(t, t.TempDir(), "brief.md", "Build the demo.\n")

	worktree := mainDir + "-demo-1"
	want := result{stdout: "Started demo-1: branch team/demo-1, worktree " + worktree +
		", tmux session baton-main-demo-1. The lead has the brief.\n"}

	got := runEnv(t.Context(), startIn(mainDir, &fakeTmux{}, slug, "--brief", brief))
	if got != want {
		t.Fatalf("baton feature start = %+v, want %+v", got, want)
	}

	feature, err := state.Store{Dir: filepath.Join(mainDir, ".git", "baton")}.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	now := flow.State{Holder: flow.RoleLead, Status: flow.StatusRunning, Cap: 5}
	if feature.Branch != "team/demo-1" || !reflect.DeepEqual(feature.Flow, now) {
		t.Errorf("feature = %+v; want branch team/demo-1 and flow %+v", feature, now)
	}

	if got := gitIn(t, worktree, exec.CommandContext(t.Context(), "git", "branch",
		"--show-current")); got != "team/demo-1\n" {
		t.Errorf("the worktree has branch %q checked out; want team/demo-1", got)
	}
}

// configPath returns the path of baton's config file in the user's config folder.
func configPath(t *testing.T) string {
	t.Helper()

	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	return filepath.Join(dir, "baton", "config.json")
}

// writeConfig writes baton's config file, with the text, until the test ends, in the home folder,
// which the test has moved into one of its own (TestMain).
func writeConfig(t *testing.T, text string) {
	t.Helper()

	path := configPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Dir(path), filepath.Base(path), text)
	t.Cleanup(func() { _ = os.Remove(path) })
}

package cli_test

import (
	"testing"
)

// The questions and reasons of baton hook guard in the orchestrator's pane (decision 54).
const (
	guardAsks = "baton's guard asks you: "
	toApprove = guardAsks + "`baton feature start` starts a feature's team on its brief. Allow " +
		"it if you approved the plan; planApproval in baton's config file turns this question off."
	orchestratorControl = "drives the user's tmux panes or the Mac's apps, which the " +
		"orchestrator never does: baton runs the panes, and `baton status` shows the features."
	orchestratorAgent = "starts another agent, which the orchestrator never does: start a " +
		"feature, and its team does the work."
	orchestratorBaton = "isn't the orchestrator's command: it runs `baton jira`, " +
		"`baton feature start`, `baton wait`, `baton answer`, `baton handover`, " +
		"`baton status`, `baton deliver` and `baton feature close`."
	toDeliver = guardAsks + "`baton deliver` pushes a feature's branch to origin, then opens " +
		"its pull request on GitHub or Bitbucket Cloud, or comments on it. Allow it if you want " +
		"the feature delivered."
	toClose = guardAsks + "`baton feature close` ends a feature's team, even mid-work: its tmux " +
		"session, with the roles' panes. It keeps the feature's branch, and its worktree unless " +
		"--remove-worktree. Allow it if you want the feature closed."
)

// asks returns the guard's question to the user about the orchestrator's outward command, which
// the guard names so.
func asks(name string) verdict {
	return verdict{"ask", guardAsks + "`" + name + "` can act outside the Mac. Allow it only if " +
		"you want it to."}
}

// forbids returns the guard's answer that denies the orchestrator's script for the command, which
// the guard names so.
func forbids(name, why string) verdict { return verdict{"deny", denied(name, why)} }

// TestGuardHookAsksTheUserAboutTheOrchestratorsCommands asks the user before the orchestrator's
// first command that can act outside the Mac, denies its first command that the guard watches,
// but baton's commands for the orchestrator and those that ask, and a script that the guard can't
// read, and lets the others run (decision 54). A denial wins over a question.
func TestGuardHookAsksTheUserAboutTheOrchestratorsCommands(t *testing.T) {
	cases := map[string]verdict{
		"git status && git log -1":               {},
		"curl -fsS https://example.com/docs":     {},
		"":                                       {},
		"git push origin HEAD":                   asks("git push"),
		"gh pr create --fill":                    asks("gh pr create"),
		"curl -d x https://h/":                   asks("curl"),
		"git status; git push && gh pr merge 7":  asks("git push"),
		"tmux send-keys -t 0 y":                  forbids("tmux", orchestratorControl),
		"osascript -e 'beep'":                    forbids("osascript", orchestratorControl),
		"claude -p 'Review it'":                  forbids("claude", orchestratorAgent),
		`"$GIT" push`:                            forbids("… push", toUnknown),
		"baton ask 'Which one?'":                 forbids("baton ask", orchestratorBaton),
		"baton hook guard":                       forbids("baton hook guard", orchestratorBaton),
		"baton pane":                             forbids("baton pane", orchestratorBaton),
		"baton feature":                          forbids("baton feature", orchestratorBaton),
		"baton":                                  forbids("baton", orchestratorBaton),
		"baton wait":                             {},
		"baton wait -h":                          {},
		"baton answer demo-1 'Keep it'":          {},
		"baton handover demo-1 --to lead":        {},
		"baton status":                           {},
		"baton jira DEMO-1":                      {},
		"baton deliver demo-1 --title 'Add it'":  {"ask", toDeliver},
		"baton deliver demo-1 && git push":       {"ask", toDeliver},
		"git push; baton deliver demo-1":         asks("git push"),
		"baton feature close demo-1":             {"ask", toClose},
		"baton feature close -remove-worktree x": {"ask", toClose},
		"git push; baton feature close demo-1":   asks("git push"),
		"baton deliver demo-1; tmux ls":          forbids("tmux", orchestratorControl),
		"baton help feature":                     {},
		"baton -h":                               {},
		"baton -help":                            {},
		"baton --help":                           {},
		"git push && tmux ls":                    forbids("tmux", orchestratorControl),
		"tmux ls; gh pr create && claude -p 'x'": forbids("tmux", orchestratorControl),
		"echo 'open":                             {"deny", unreadable},
	}

	dir := t.TempDir()

	for script, want := range cases {
		t.Run(script, func(t *testing.T) {
			args := map[string]any{"command": script, "description": "Run it", "mode": "sync"}
			input := guardInput(t, dir, "bash", args)
			wantAnswer(t, guardHook(t, dir, orchestratorPane(), input), want)
		})
	}
}

// TestGuardHookAsksTheUserBeforeAFeatureStarts asks the user before the orchestrator's baton
// feature start while plan approval is on, as it is by default, and lets it run when the config
// turns it off; the guard's other answers stand either way (decision 54).
func TestGuardHookAsksTheUserBeforeAFeatureStarts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mainDir := repoNamed(t, "main")

	const start = "baton feature start demo-1 --brief /tmp/brief.md"

	cases := []struct {
		name   string
		config string // The config file's text, or "" for none.
		script string
		want   verdict
	}{
		{"by default", "", start, verdict{"ask", toApprove}},
		{"with plan approval on", `{"planApproval": true}`, start, verdict{"ask", toApprove}},
		{"with plan approval off", `{"planApproval": false}`, start, verdict{}},
		{"with help", "", "baton feature start -h", verdict{"ask", toApprove}},
		{"before an outward command", "", start + " && git push", verdict{"ask", toApprove}},
		{"after an outward command", "", "git push; " + start, asks("git push")},
		{
			"with plan approval off, and an outward command", `{"planApproval": false}`,
			start + " && git push", asks("git push"),
		},
		{
			"before a command that the guard denies", "", start + " && tmux ls",
			forbids("tmux", orchestratorControl),
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.config != "" {
				writeConfig(t, test.config)
			}

			input := guardInput(t, mainDir, "bash", map[string]any{"command": test.script})
			wantAnswer(t, guardHook(t, mainDir, orchestratorPane(), input), test.want)
		})
	}
}

// TestGuardHookLetsTheOrchestratorsOtherToolsRun lets the orchestrator's tools other than bash,
// create and edit run, whatever their arguments: they read, look after its shells, or talk to
// the user.
func TestGuardHookLetsTheOrchestratorsOtherToolsRun(t *testing.T) {
	mainDir := repoNamed(t, "main")

	cases := map[string]any{
		"view":         map[string]any{"path": mainDir + "/.git/config"},
		"grep":         map[string]any{"pattern": "push", "path": mainDir},
		"web_fetch":    map[string]any{"url": "https://example.com/?q=push"},
		"read_bash":    map[string]any{"shellId": "7", "delay": 5},
		"ask_user":     map[string]any{"message": "git push?"},
		"nudge-report": map[string]any{"note": "Started demo-1."},
	}

	for tool, args := range cases {
		t.Run(tool, func(t *testing.T) {
			input := guardInput(t, mainDir, tool, args)
			wantGuard(t, guardHook(t, mainDir, orchestratorPane(), input), "")
		})
	}
}

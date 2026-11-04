package cli_test

import (
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
)

// session is the ID of the copilot session in the hook's input, which names its folder.
const session = "0b5d2c4e"

// The reasons that baton hook guard denies a role's tool with (decision 52).
const (
	guardStopped = "baton's guard stopped this call, and none of it ran. "
	toOutward    = "can act outside the Mac, which only the orchestrator does, with the user's " +
		"approval. Keep your work local, and say in your handover note what should go out."
	toControl = "drives the user's tmux panes or the Mac's apps, which a role never does."
	toAgent   = "starts another agent, which a role never does: do the work yourself."
	toBaton   = "isn't a role's command: a role runs `baton wait`, `baton handover`, " +
		"`baton ask` and `baton status`."
	toUnknown = "runs a program that only the run decides, which the guard can't check: name " +
		"the program in the script itself."
	// unreadable denies the script echo 'open, which the guard can't read, in every pane.
	unreadable = guardStopped + "The guard can't read the script (1:6: reached EOF without " +
		"closing quote `'`): write it in plain bash."
)

// denied returns the reason that denies a role's script for the command, which the guard names so.
func denied(name, why string) string { return guardStopped + "`" + name + "` " + why }

// TestGuardHookDeniesARoleWhatTheGuardWatches denies a role's bash script for its first command
// that the guard watches, but baton's commands that a role runs, and a script that the guard can't
// read; it lets the others run.
func TestGuardHookDeniesARoleWhatTheGuardWatches(t *testing.T) {
	cases := map[string]string{
		"go test ./... && git commit -am 'Add the demo'": "",
		"curl -fsS https://example.com/docs":             "",
		"":                                               "",
		"git push origin HEAD":                           denied("git push", toOutward),
		"gh pr create --fill":                            denied("gh pr create", toOutward),
		"curl -d x https://h/":                           denied("curl", toOutward),
		"tmux send-keys -t 0 y":                          denied("tmux", toControl),
		"osascript -e 'beep'":                            denied("osascript", toControl),
		"claude -p 'Review it'":                          denied("claude", toAgent),
		`"$GIT" push`:                                    denied("… push", toUnknown),
		"baton answer demo-1 y":                          denied("baton answer", toBaton),
		"baton feature start x":                          denied("baton feature start", toBaton),
		"baton hook stop":                                denied("baton hook stop", toBaton),
		"baton pane":                                     denied("baton pane", toBaton),
		"baton jira DEMO-1":                              denied("baton jira", toBaton),
		"baton deliver demo-1":                           denied("baton deliver", toBaton),
		"baton":                                          denied("baton", toBaton),
		`baton "$verb"`:                                  denied("baton …", toBaton),
		"baton wait":                                     "",
		"baton wait -h":                                  "",
		"baton handover --to qa":                         "",
		"baton ask 'Which one?'":                         "",
		"baton status":                                   "",
		"baton help":                                     "",
		"baton -h":                                       "",
		"baton -help":                                    "",
		"baton --help":                                   "",
		"git status && baton wait && git push && tmux ls": denied("git push", toOutward),
		"echo 'open": unreadable,
	}

	dir := t.TempDir()

	for script, want := range cases {
		t.Run(script, func(t *testing.T) {
			args := map[string]any{"command": script, "description": "Run it", "mode": "sync"}
			input := guardInput(t, dir, "bash", args)
			wantGuard(t, guardHook(t, dir, paneOf(flow.RoleEngineer), input), want)
		})
	}
}

// TestGuardHookLetsARolesOtherToolsRun lets a role's tools other than bash, create and edit run,
// whatever their arguments, since they read, or look after the role's shells: even a view of
// baton's folder.
func TestGuardHookLetsARolesOtherToolsRun(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	cases := map[string]any{
		"view":      map[string]any{"path": store.Dir},
		"grep":      map[string]any{"pattern": "push", "path": store.Dir},
		"glob":      map[string]any{"pattern": "**/*.md", "path": store.Dir},
		"web_fetch": map[string]any{"url": "https://example.com/?q=push"},
		"read_bash": map[string]any{"shellId": "7", "delay": 5},
		"stop_bash": map[string]any{"shellId": "7"},
		"list_bash": map[string]any{},
		"task":      "git push",
	}

	for tool, args := range cases {
		t.Run(tool, func(t *testing.T) {
			input := guardInput(t, linked, tool, args)
			wantGuard(t, guardHook(t, linked, paneOf(flow.RoleQA), input), "")
		})
	}
}

// guardHook runs baton hook guard in the pane, from the folder, with the hook's input.
func guardHook(t *testing.T, dir string, env map[string]string, input string) result {
	t.Helper()

	return runEnv(t.Context(), guardEnv(dir, env, input))
}

// guardEnv returns the environment of baton hook guard in the pane, from the folder, with the
// hook's input.
func guardEnv(dir string, env map[string]string, input string) cli.Env {
	hook := hookEnv(dir, env, input)
	hook.Args = []string{"hook", "guard"}

	return hook
}

// guardInput returns the input that copilot gives its preToolUse hook when a pane's copilot, in
// the session, calls the tool with the arguments in the folder, if any.
func guardInput(t *testing.T, dir, tool string, args any) string {
	t.Helper()

	input := map[string]any{
		"sessionId": session, "timestamp": 1767225600000, "toolName": tool, "toolArgs": args,
	}
	if dir != "" {
		input["cwd"] = dir
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// wantGuard checks that baton hook guard let the tool run, without a word, if reason is "", and
// otherwise denied it with the reason.
func wantGuard(t *testing.T, got result, reason string) {
	t.Helper()
	wantAnswer(t, got, verdict{"deny", reason})
}

// verdict is baton hook guard's answer to a tool: deny or ask, with the reason, or none.
type verdict struct{ decision, reason string }

// wantAnswer checks that baton hook guard let the tool run, without a word, if the reason is "",
// and otherwise gave its answer with the reason: a JSON object, on a line of its own.
func wantAnswer(t *testing.T, got result, want verdict) {
	t.Helper()

	if want.reason == "" {
		if got != (result{}) {
			t.Errorf("got %+v; want exit code 0 and no output", got)
		}

		return
	}

	var answer struct {
		Decision string `json:"permissionDecision"`
		Reason   string `json:"permissionDecisionReason"`
	}

	if got.code != 0 || got.stderr != "" || !strings.HasSuffix(got.stdout, "}\n") ||
		json.Unmarshal([]byte(got.stdout), &answer, json.RejectUnknownMembers(true)) != nil {
		t.Fatalf("got %+v; want exit code 0 and a JSON object", got)
	}

	if answer.Decision != want.decision || answer.Reason != want.reason {
		t.Errorf("got %+v; want the decision %s, with the reason %q", answer, want.decision,
			want.reason)
	}
}

package cli_test

import (
	"encoding/json/v2"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"github.com/codedogapp/baton/internal/flow"
)

// TestSessionHookKeepsTheOrchestratorsSession has the sessionStart hook keep the session that
// starts in the orchestrator's pane, in baton's folder of the copilot's repo, which it finds from
// the copilot's folder in the input, or from its own folder without one (decision 68).
func TestSessionHookKeepsTheOrchestratorsSession(t *testing.T) {
	mainDir, linked := repos(t)

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	cases := []struct {
		name  string
		dir   string // The hook's own folder.
		input string // The copilot's folder in the input, if any.
	}{
		{"the copilot's folder", outside, linked},
		{"no folder", linked, ""},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			session := uuid.New().String()

			got := sessionHook(
				t,
				test.dir,
				orchestratorPane(),
				sessionStarts(t, test.input, session),
			)
			if got != (result{}) {
				t.Errorf("got %+v; want exit code 0 and no output", got)
			}

			if kept := orchestratorSession(t, mainDir); kept != session {
				t.Errorf("baton keeps session %s; want %s", kept, session)
			}
		})
	}
}

// TestSessionHookLeavesARolesSessionToBaton does nothing in a role's pane, whose session baton
// gives its copilot, and makes no file in baton's folder (decision 66).
func TestSessionHookLeavesARolesSessionToBaton(t *testing.T) {
	mainDir, linked := repos(t)
	started(t, mainDir, fresh())

	input := sessionStarts(t, linked, uuid.New().String())
	if got := sessionHook(t, linked, paneOf(flow.RoleQA), input); got != (result{}) {
		t.Errorf("got %+v; want exit code 0 and no output", got)
	}

	_, err := os.Stat(filepath.Join(mainDir, ".git", "baton", "orchestrator.json"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("got %v; want no orchestrator's file", err)
	}
}

// TestSessionHookFails fails, with the reason on stderr and nothing on stdout, when it can't keep
// the orchestrator's session; copilot then goes on, and the pane's next start resumes the session
// that baton kept before.
func TestSessionHookFails(t *testing.T) {
	mainDir, linked := repos(t)

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	damaged := repoNamed(t, "damaged")
	newer := write(t, mkdir(t, filepath.Join(damaged, ".git", "baton")), "orchestrator.json",
		`{"format": 2}`)

	input := sessionStarts(t, linked, uuid.New().String())
	cases := []struct {
		name  string
		args  []string
		env   map[string]string
		input string
		want  string // The start of the reason, after "baton hook session: ".
	}{
		{
			"with an argument",
			[]string{"now"},
			orchestratorPane(), input,
			`unexpected argument "now"`,
		},
		{"without input", nil, orchestratorPane(), "", "read the hook's input: EOF"},
		{
			"in a role's pane without input",
			nil,
			paneOf(flow.RoleQA),
			"",
			"read the hook's input: EOF",
		},
		{
			"with a session's ID that isn't one", nil, orchestratorPane(),
			`{"sessionId": "0b5d2c4e", "cwd": "` + linked + `"}`, "read the hook's input: json: ",
		},
		{
			"without a session's ID", nil, orchestratorPane(), `{"cwd": "` + linked + `"}`,
			"keep the orchestrator's session: want its ID, not the nil UUID\n",
		},
		{
			"outside a pane", nil, nil, input,
			"want BATON_ROLE: run it in a pane that baton started, which names its role there\n",
		},
		{
			"outside a git repo", nil, orchestratorPane(),
			sessionStarts(t, outside, uuid.New().String()),
			"find the git repo: fatal: not a git repository",
		},
		{
			"with an orchestrator's file that it can't read", nil, orchestratorPane(),
			sessionStarts(t, damaged, uuid.New().String()),
			"orchestrator file " + newer + ": format 2 is newer than this baton's, 1: run the " +
				"newer baton\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hook := hookEnv(linked, test.env, test.input)
			hook.Args = append([]string{"hook", "session"}, test.args...)
			got := runEnv(t.Context(), hook)

			want := "baton hook session: " + test.want
			if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("got %+v; want exit code 1 and %q", got, want)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(mainDir, ".git", "baton")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("got %v; want no baton folder", err)
	}
}

// sessionHook runs baton hook session in the pane, from the folder, with the hook's input.
func sessionHook(t *testing.T, dir string, env map[string]string, input string) result {
	t.Helper()

	hook := hookEnv(dir, env, input)
	hook.Args = []string{"hook", "session"}

	return runEnv(t.Context(), hook)
}

// sessionStarts returns the input that copilot gives its sessionStart hook at the first prompt of
// the new session with the ID, in the folder, if any (fact 50).
func sessionStarts(t *testing.T, dir, session string) string {
	t.Helper()

	input := map[string]any{
		"sessionId": session, "timestamp": 1767225600000, "source": "new",
		"initialPrompt": "Plan the demo.",
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

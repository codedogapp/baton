package cli_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
)

// TestStopHookReadsItsInput lets a copilot stop once the hook has sent its turn back, even outside
// a pane, since copilot asks again when that turn ends, and finds the copilot's repo from the
// copilot's folder in the input, or from its own folder without one.
func TestStopHookReadsItsInput(t *testing.T) {
	mainDir, linked := repos(t)
	started(t, mainDir, fresh())

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	cases := []struct {
		name  string
		dir   string // The hook's own folder.
		env   map[string]string
		input string
		want  string // The reason, or "" to let the copilot stop.
	}{
		{"a turn sent back", linked, paneOf(flow.RoleQA), hookInput(t, linked, true), ""},
		{"a turn sent back, outside a pane", outside, nil, hookInput(t, outside, true), ""},
		{"the copilot's folder", outside, paneOf(flow.RoleQA), hookInput(t, linked, false), toWait},
		{"no folder", linked, paneOf(flow.RoleQA), hookInput(t, "", false), toWait},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			wantStop(t, stopHook(t, test.dir, test.env, test.input), test.want)
		})
	}
}

// TestStopHookLetsTheOrchestratorStopWithoutFeatures lets the orchestrator stop in a repo where
// baton has started no feature, since no work can come to it there, and makes no folder there.
func TestStopHookLetsTheOrchestratorStopWithoutFeatures(t *testing.T) {
	mainDir, linked := repos(t)

	wantStop(t, stopHook(t, linked, orchestratorPane(), hookInput(t, linked, false)), "")

	_, err := os.Stat(filepath.Join(mainDir, ".git", "baton"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("got %v; want no baton folder", err)
	}
}

// TestStopHookReadsOneObject reads the hook's input, a JSON object, without waiting for the end of
// the input, which copilot might keep open.
func TestStopHookReadsOneObject(t *testing.T) {
	mainDir, linked := repos(t)
	started(t, mainDir, fresh())

	hook := hookEnv(linked, paneOf(flow.RoleQA), "")
	hook.Stdin = &openInput{rest: hookInput(t, linked, false)}

	wantStop(t, runEnv(t.Context(), hook), toWait)
}

// openInput is input that stays open: a read after its end fails, where a real one would wait.
type openInput struct{ rest string }

func (input *openInput) Read(buf []byte) (int, error) {
	if input.rest == "" {
		return 0, errors.New("read after the end of the input, which stays open")
	}

	read := copy(buf, input.rest)
	input.rest = input.rest[read:]

	return read, nil
}

// TestStopHookFailsAndLetsTheCopilotStop fails, with the reason on stderr and nothing on stdout,
// when it can't tell whether the copilot may stop; copilot then lets it stop.
func TestStopHookFailsAndLetsTheCopilotStop(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	// The engineer's mark, a link out of baton's folder, which baton doesn't follow.
	mark := filepath.Join(store.Dir, "features", slug, "engineer.wait")
	if err := os.Symlink(outside, mark); err != nil {
		t.Fatal(err)
	}

	// demo-2's file, which isn't JSON, so the orchestrator's hook can't list the features.
	broken := filepath.Join(store.Dir, "features", "demo-2")
	if err := os.Mkdir(broken, 0o700); err != nil {
		t.Fatal(err)
	}

	write(t, broken, "feature.json", "{")

	inQA, input := paneOf(flow.RoleQA), hookInput(t, linked, false)
	cases := []struct {
		name  string
		args  []string
		env   map[string]string
		input string
		want  string // The start of the reason, after "baton hook stop: ".
	}{
		{"with an argument", []string{"now"}, inQA, input, `unexpected argument "now"`},
		{"without input", nil, inQA, "", "read the hook's input: EOF"},
		{"with input that isn't JSON", nil, inQA, "stop", "read the hook's input: jsontext: "},
		{"with input that isn't an object", nil, inQA, "[]", "read the hook's input: json: "},
		{
			"outside a pane", nil, nil, input,
			"want BATON_ROLE: run it in a pane that baton started, which names its role there\n",
		},
		{
			"in no feature", nil,
			map[string]string{"BATON_ROLE": "qa", "BATON_FEATURE": "demo-3"},
			input, "no feature demo-3\n",
		},
		{
			"with a bad slug", nil,
			map[string]string{"BATON_ROLE": "qa", "BATON_FEATURE": "Demo"},
			input, `slug "Demo": want words of lowercase letters and digits`,
		},
		{
			"with a mark that it can't look at", nil, paneOf(flow.RoleEngineer), input,
			"look for the engineer's wait: openat features/demo-1/engineer.wait: ",
		},
		{
			"with features that it can't list", nil, orchestratorPane(),
			hookInput(t, mainDir, false), "feature file ",
		},
		{
			"outside a git repo", nil, inQA, hookInput(t, outside, false),
			"find the git repo: fatal: not a git repository",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hook := hookEnv(linked, test.env, test.input)
			hook.Args = append(hook.Args, test.args...)
			got := runEnv(t.Context(), hook)

			want := "baton hook stop: " + test.want
			if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("got %+v; want exit code 1 and %q", got, want)
			}
		})
	}
}

// TestStopHookReportsAnOutputItCantWrite fails when it can't write its answer, which copilot then
// doesn't get, so that copilot lets the copilot stop.
func TestStopHookReportsAnOutputItCantWrite(t *testing.T) {
	mainDir, linked := repos(t)
	started(t, mainDir, fresh())

	var stderr strings.Builder

	hook := hookEnv(linked, paneOf(flow.RoleQA), hookInput(t, linked, false))
	hook.Stdout, hook.Stderr = failing{}, &stderr

	code := cli.Run(t.Context(), hook)
	if got := stderr.String(); code != 1 ||
		got != "baton hook stop: jsontext: write error: output closed\n" {
		t.Errorf("exit code %d and %q; want 1 and the output's error", code, got)
	}
}

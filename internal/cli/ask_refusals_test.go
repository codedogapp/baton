package cli_test

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestAskAndAnswerRefuseAndWriteNothing refuses the questions that the orchestrator would never
// answer, and the answers that no question waits for. demo-2's lead has had its answer. Each runs
// in a bubble, where an ask that waits by mistake stops after an hour of the bubble's time.
func TestAskAndAnswerRefuseAndWriteNothing(t *testing.T) {
	mainDir, _ := repos(t)
	store := started(t, mainDir, fresh())
	add(t, store, state.Feature{Slug: "demo-2", Flow: fresh()})

	edit := state.Entry{Event: state.Event{Kind: "edit", Role: flow.RoleOrchestrator}}
	if _, err := store.Update("demo-2", edit, func(feature *state.Feature) error {
		feature.Question = state.Question{From: flow.RoleLead, Text: question, Answer: "demo.txt"}

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	lead, orchestrator := paneOf(flow.RoleLead), orchestratorPane()
	elsewhere := map[string]string{"BATON_ROLE": "lead", "BATON_FEATURE": "demo-3"}
	wantQuestion := `want the question: baton ask "<question>"` + "\n"
	wantAnswer := `want the answer: baton answer <slug> "<answer>"` + "\n"
	cases := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"no question", lead, []string{"ask"}, wantQuestion},
		{"a blank question", lead, []string{"ask", " \n"}, wantQuestion},
		{
			"an unquoted question", lead,
			[]string{"ask", "Which", "file?"},
			`unexpected argument "file?": quote each text for the shell` + "\n",
		},
		{
			"a question outside a pane", nil,
			[]string{"ask", question},
			"want BATON_ROLE: run it in a pane that baton started, which names its role there\n",
		},
		{
			"the orchestrator's question", orchestrator,
			[]string{"ask", question},
			"the roles ask the orchestrator; the orchestrator asks the user\n",
		},
		{
			"the question of a role that doesn't hold the feature", paneOf(flow.RoleQA),
			[]string{"ask", question},
			"lead holds demo-1, and only the holder asks\n",
		},
		{"a question in no feature", elsewhere, []string{"ask", question}, "no feature demo-3\n"},
		{
			"no slug", orchestrator,
			[]string{"answer"},
			`want the feature's slug: baton answer <slug> "<answer>"` + "\n",
		},
		{"no answer", orchestrator, []string{"answer", "demo-1"}, wantAnswer},
		{"a blank answer", orchestrator, []string{"answer", "demo-1", ""}, wantAnswer},
		{
			"an unquoted answer", orchestrator,
			[]string{"answer", "demo-1", "demo", ".txt"},
			`unexpected argument ".txt": quote each text for the shell` + "\n",
		},
		{
			"an answer outside a pane", nil,
			[]string{"answer", "demo-1", "demo.txt"},
			"want BATON_ROLE: run it in a pane that baton started, which names its role there\n",
		},
		{
			"a role's answer", lead,
			[]string{"answer", "demo-1", "demo.txt"},
			"only the orchestrator answers: the lead asks with baton ask\n",
		},
		{
			"an answer with no question", orchestrator,
			[]string{"answer", "demo-1", "demo.txt"},
			"no question waits for its answer in demo-1\n",
		},
		{
			"a second answer", orchestrator,
			[]string{"answer", "demo-2", "demo.txt"},
			"no question waits for its answer in demo-2\n",
		},
		{
			"an answer in no feature", orchestrator,
			[]string{"answer", "demo-3", "demo.txt"},
			"no feature demo-3\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before := snapshot(t, store.Dir)

			synctest.Test(t, func(t *testing.T) {
				got, want := runFor(t, mainDir, test.env, test.args...), result{
					code: 1, stderr: "baton " + test.args[0] + ": " + test.want,
				}
				if got != want {
					t.Errorf("got %+v; want %+v", got, want)
				}
			})

			if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed to %v", after)
			}
		})
	}
}

// TestAskAndAnswerRefuseOutsideARepo refuses a question and an answer that have no git repo, and so
// no feature, to go to.
func TestAskAndAnswerRefuseOutsideARepo(t *testing.T) {
	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	for _, test := range []struct {
		env  map[string]string
		args []string
	}{
		{paneOf(flow.RoleLead), []string{"ask", question}},
		{orchestratorPane(), []string{"answer", "demo-1", "demo.txt"}},
	} {
		t.Run(test.args[0], func(t *testing.T) {
			got := run(t, outside, test.env, test.args...)

			want := "baton " + test.args[0] + ": find the git repo: fatal: not a git repository"
			if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("got %+v; want exit code 1 and %q", got, want)
			}
		})
	}
}

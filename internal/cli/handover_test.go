package cli_test

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestHandoverPassesTheFeatureAlong walks a round that the reviewer rejects, from a subfolder of
// the main worktree and from the linked worktree, which share baton's folder.
func TestHandoverPassesTheFeatureAlong(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	sub := filepath.Join(mainDir, "docs")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		dir      string
		role     flow.Role
		relative bool // Whether --note gets the note's path relative to the folder.
		verdict  []string
		want     string
	}{
		{sub, flow.RoleLead, false, nil, "engineer: running, round 1 of 3"},
		{linked, flow.RoleEngineer, true, nil, "qa: running, round 1 of 3"},
		{
			linked,
			flow.RoleQA,
			true,
			[]string{"--verdict", "approve"},
			"reviewer: running, round 1 of 3",
		},
		{
			linked,
			flow.RoleReviewer,
			true,
			[]string{"--verdict=reject"},
			"engineer: running, round 2 of 3",
		},
	}

	for _, step := range steps {
		note := write(t, step.dir, "note.md", "From the "+string(step.role)+".\n")
		if step.relative {
			note = "note.md"
		}

		got := run(t, step.dir, paneOf(step.role),
			append([]string{"handover", "--note", note}, step.verdict...)...)

		want := result{stdout: "Handed demo-1 over to " + step.want + ".\n"}
		if got != want {
			t.Fatalf("the %s's handover: got %+v; want %+v", step.role, got, want)
		}
	}

	feature, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	want := flow.State{Holder: flow.RoleEngineer, Status: flow.StatusRunning, Round: 2, Cap: 3}
	if feature.Flow != want {
		t.Errorf("flow = %+v; want %+v", feature.Flow, want)
	}

	wantJournal(t, store, []logged{
		{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
		{"handover lead  02-lead-to-engineer.md", "From the lead.\n"},
		{"handover engineer  03-engineer-to-qa.md", "From the engineer.\n"},
		{"handover qa approve 04-qa-to-reviewer.md", "From the qa.\n"},
		{"handover reviewer reject 05-reviewer-to-engineer.md", "From the reviewer.\n"},
	})
}

func TestHandoverReportsAnEscalation(t *testing.T) {
	mainDir, linked := repos(t)
	started(t, mainDir, flow.State{
		Holder: flow.RoleReviewer, Status: flow.StatusRunning, Round: 4, Cap: 3, Extra: 1,
	})

	note := write(t, linked, "note.md", "Still broken.\n")
	args := []string{"handover", "--note", note, "--verdict", "reject"}

	got := run(t, linked, paneOf(flow.RoleReviewer), args...)

	want := result{stdout: "Handed demo-1 over to orchestrator: escalated, round 4 of 4.\n"}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

func TestHandoverRefusesAndWritesNothing(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	before := snapshot(t, store.Dir)

	notes, outside := t.TempDir(), t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	note := write(t, notes, "note.md", "Done.\n")
	blank := write(t, notes, "blank.md", " \n\t\n")
	lead := paneOf(flow.RoleLead)
	roleless := "want BATON_ROLE: run it in a pane that baton started, which names its role there\n"

	cases := []struct {
		name string
		dir  string
		env  map[string]string
		args []string
		want string // The start of the reason, after "baton handover: ".
	}{
		{"outside a pane", linked, nil, []string{"--note", note}, roleless},
		{
			"without its feature", linked,
			map[string]string{"BATON_ROLE": "lead"},
			nil,
			"want BATON_FEATURE: the lead's pane names its feature there\n",
		},
		{"without its role", linked, map[string]string{"BATON_FEATURE": slug}, nil, roleless},
		{"without a note", linked, lead, nil, "want --note <file>: the note for the next role\n"},
		{
			"with a missing note", linked, lead,
			[]string{"--note", "gone.md"},
			"read the note: open " + filepath.Join(linked, "gone.md") + ": ",
		},
		{
			"with a blank note", linked, lead,
			[]string{"--note", blank},
			"note " + blank + " is blank: it's the next role's work\n",
		},
		{
			"from a role that doesn't hold it", linked, paneOf(flow.RoleEngineer),
			[]string{"--note", note},
			"lead holds the feature, not engineer\n",
		},
		{
			"with a verdict from the lead", linked, lead,
			[]string{"--note", note, "--verdict", "approve"},
			"lead hands over without a verdict\n",
		},
		{
			"of another feature", linked,
			map[string]string{"BATON_ROLE": "lead", "BATON_FEATURE": "demo-2"},
			[]string{"--note", note},
			"no feature demo-2\n",
		},
		{
			"of a path for a slug", linked,
			map[string]string{"BATON_ROLE": "lead", "BATON_FEATURE": "../x"},
			[]string{"--note", note},
			`slug "../x": want words`,
		},
		{
			"with an argument too many", linked, lead,
			[]string{"--note", note, "now"},
			"unexpected argument \"now\"\n",
		},
		{
			"outside a git repo", outside, lead,
			[]string{"--note", note},
			"find the git repo: fatal: not a git repository",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := run(t, test.dir, test.env, append([]string{"handover"}, test.args...)...)

			want := "baton handover: " + test.want
			if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("got %+v; want exit code 1 and %q", got, want)
			}

			if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed to %v", after)
			}
		})
	}
}

// logged is an event of the feature's journal, as its kind, role, verdict and note, and the
// note's text.
type logged struct {
	event string
	note  string
}

// wantJournal checks the feature's events, in order, and their notes, if any.
func wantJournal(t *testing.T, store state.Store, want []logged) {
	t.Helper()

	folder := filepath.Join(store.Dir, "features", slug)

	data, err := os.ReadFile(filepath.Clean(filepath.Join(folder, "events.ndjson")))
	if err != nil {
		t.Fatal(err)
	}

	var got []logged

	for line := range strings.Lines(string(data)) {
		var event state.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}

		var note []byte
		if event.Note != "" {
			note, err = os.ReadFile(filepath.Clean(filepath.Join(folder, "notes", event.Note)))
			if err != nil {
				t.Fatal(err)
			}
		}

		got = append(got, logged{
			event: fmt.Sprintf("%s %s %s %s", event.Kind, event.Role, event.Verdict, event.Note),
			note:  string(note),
		})
	}

	if !slices.Equal(got, want) {
		t.Errorf("journal:\n%q\nwant:\n%q", got, want)
	}
}

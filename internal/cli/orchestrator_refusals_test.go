package cli_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestOrchestratorsWaitRefusesWhenNoFeatureRuns refuses the orchestrator's waits that could never
// end, and writes nothing: without features, and once the orchestrator holds them all. A broken
// feature fails the wait too.
func TestOrchestratorsWaitRefusesWhenNoFeatureRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		refused := result{code: 1, stderr: noneRuns}

		if got := runFor(t, mainDir, orchestratorPane(), "wait"); got != refused {
			t.Errorf("without features: got %+v; want %+v", got, refused)
		}

		folder := filepath.Join(mainDir, ".git", "baton")
		if _, err := os.Stat(folder); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("baton's folder: got %v; want none", err)
		}

		store := started(t, mainDir, flow.State{
			Holder: flow.RoleLead, Status: flow.StatusRunning, Round: 1, Cap: 3,
		})
		handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

		if got := runFor(t, mainDir, orchestratorPane(), "wait"); got.code != 0 {
			t.Fatalf("the wait for the final report: %+v", got)
		}

		before := snapshot(t, store.Dir)

		if got := runFor(t, mainDir, orchestratorPane(), "wait"); got != refused {
			t.Errorf("with a ready feature: got %+v; want %+v", got, refused)
		}

		if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
			t.Errorf("baton's folder changed to %v", after)
		}

		broken := write(t, filepath.Join(store.Dir, "features", slug), "feature.json",
			`{"format": 2}`)
		want := result{
			code: 1,
			stderr: "baton wait: feature file " + broken + ": format 2 is newer than this " +
				"baton's, 1: run the newer baton\n",
		}

		if got := runFor(t, mainDir, orchestratorPane(), "wait"); got != want {
			t.Errorf("with a broken feature: got %+v; want %+v", got, want)
		}
	})
}

// TestOrchestratorsHandoverRefusesAndWritesNothing refuses the handovers that name no feature
// or no next role, or that the flow doesn't allow, and a pick of the next role by another role.
func TestOrchestratorsHandoverRefusesAndWritesNothing(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	add(t, store, state.Feature{Slug: "demo-2", Flow: flow.State{
		Holder: flow.RoleOrchestrator, Status: flow.StatusReady, Round: 1, Cap: 3,
	}})

	before := snapshot(t, store.Dir)
	note := write(t, t.TempDir(), "note.md", "Done.\n")

	cases := []struct {
		name string
		env  map[string]string
		args []string
		want string // The reason, after "baton handover: ".
	}{
		{
			"without a slug", orchestratorPane(),
			[]string{"--to", "lead", "--note", note},
			"want the feature's slug: baton handover <slug> --to <role> --note <file>\n",
		},
		{
			"without the next role", orchestratorPane(),
			[]string{"demo-2", "--note", note},
			"want --to lead, with a new brief, or, after an escalation, --to engineer\n",
		},
		{
			"with an argument too many", orchestratorPane(),
			[]string{"demo-2", "now", "--to", "lead", "--note", note},
			"unexpected argument \"now\"\n",
		},
		{
			"with a verdict", orchestratorPane(),
			[]string{"demo-2", "--to", "lead", "--verdict", "approve", "--note", note},
			"orchestrator hands over without a verdict\n",
		},
		{
			"of a ready feature to the engineer", orchestratorPane(),
			[]string{"demo-2", "--to", "engineer", "--note", note},
			`the orchestrator can't hand a feature that is ready to "engineer": ` +
				"it goes to the lead or, after an escalation, to the engineer\n",
		},
		{
			"of a running feature", orchestratorPane(),
			[]string{slug, "--note", note, "--to", "lead"},
			"lead holds the feature, not orchestrator\n",
		},
		{
			"of a feature that the lead names", paneOf(flow.RoleLead),
			[]string{slug, "--note", note},
			"unexpected argument \"demo-1\"\n",
		},
		{
			"to the role that the lead picks", paneOf(flow.RoleLead),
			[]string{"--to", "engineer", "--note", note},
			"lead can't pick the next role: baton picks it\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := run(t, linked, test.env, append([]string{"handover"}, test.args...)...)

			want := result{code: 1, stderr: "baton handover: " + test.want}
			if got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed to %v", after)
			}
		})
	}
}

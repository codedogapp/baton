package cli_test

import (
	"encoding/json/v2"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestHandoverRefusesWhereTheGateCantRun refuses the engineer's handover where the repo's gate
// can't run, and a handover that the flow refuses before its gate runs: no gate runs, and baton's
// folder stays as it was.
func TestHandoverRefusesWhereTheGateCantRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	marker := filepath.Join(t.TempDir(), "ran")
	mark := "touch '" + marker + "'"

	marking, err := json.Marshal(map[string]string{"gate": mark})
	if err != nil {
		t.Fatal(err)
	}

	gone := filepath.Join(t.TempDir(), "gone")

	engineer := flow.State{Holder: flow.RoleEngineer, Status: flow.StatusRunning, Round: 1, Cap: 3}
	question := state.Question{From: flow.RoleEngineer, Text: "Which file?"}

	cases := []struct {
		name    string
		config  string        // The config file's text.
		feature state.Feature // The feature, with the main worktree, unless it names another.
		stdout  string
		want    string // The reason, after "baton handover: ".
	}{
		{"without a gate", `{"roundCap": 2}`, state.Feature{Flow: engineer}, "", noGate(t)},
		{
			"with a mistake in the config file", `{"gate": "true", "roundCap": -1}`,
			state.Feature{Flow: engineer},
			"",
			"config file " + configPath(t) + ": roundCap -1: want 1 or more\n",
		},
		{
			"without a worktree", string(marking),
			state.Feature{Worktree: "-", Flow: engineer},
			"", "feature demo-1 names no worktree, where its gate runs\n",
		},
		{
			"whose worktree is gone", string(marking),
			state.Feature{Worktree: gone, Flow: engineer},
			ranGate(gone, mark, ""),
			`gate "` + mark + `" failed: chdir ` + gone + ": no such file or directory: the " +
				"feature stays with you until it passes\n",
		},
		{
			"from a role that doesn't hold it", string(marking),
			state.Feature{Flow: fresh()},
			"", "lead holds the feature, not engineer\n",
		},
		{
			"while the holder's question waits", string(marking),
			state.Feature{Flow: engineer, Question: question},
			"",
			"the engineer's question \"Which file?\" waits for its answer: wait for it, " +
				"then hand over\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			writeConfig(t, test.config)

			mainDir, linked := repos(t)
			store := state.Store{Dir: filepath.Join(mainDir, ".git", "baton")}

			feature := test.feature
			feature.Slug = slug

			switch feature.Worktree {
			case "-":
				feature.Worktree = ""
			case "":
				feature.Worktree = mainDir
			}

			add(t, store, feature)
			before := snapshot(t, store.Dir)

			note := write(t, linked, "note.md", "Done.\n")
			got := run(t, linked, paneOf(flow.RoleEngineer), "handover", "--note", note)

			want := result{code: 1, stdout: test.stdout, stderr: "baton handover: " + test.want}
			if got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Errorf("the gate ran: %v", err)
			}

			if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed to %v", after)
			}
		})
	}
}

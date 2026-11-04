package cli_test

import (
	"encoding/json/v2"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// checked is a gate that says where it ran: in the folder named main, the main worktree, which
// started gives each feature as its worktree.
const checked = `printf 'checked in %s\n' "${PWD##*/}"`

// withGate gives the test a home folder of its own, with baton's config file, which gives the
// gate alone.
func withGate(t *testing.T, command string) {
	t.Helper()

	text, err := json.Marshal(map[string]string{"gate": command})
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	writeConfig(t, string(text))
}

// noGate returns baton's reason to refuse a repo without a gate, which names the config file.
func noGate(t *testing.T) string {
	t.Helper()

	return `no gate for this repo: set "gate" in ` + configPath(t) + " to the command that " +
		`checks the repo's code, such as "mise run check", or to "true" for none` + "\n"
}

// ranGate returns what baton prints when the gate runs in the folder: its own line, then the
// gate's output.
func ranGate(dir, command, output string) string {
	return "Running the gate in " + dir + ": " + command + "\n" + output
}

// TestHandoverRunsTheGate hands the feature over at each step of the flow. The engineer's
// handover, QA's approval and the lead's final report run the repo's gate first, in the feature's
// worktree, wherever the role runs baton, and the journal logs the gate with the handover. The
// other handovers run no gate (decision 56).
func TestHandoverRunsTheGate(t *testing.T) {
	withGate(t, checked)

	running := func(holder flow.Role, round int) flow.State {
		return flow.State{Holder: holder, Status: flow.StatusRunning, Round: round, Cap: 3}
	}
	escalated := flow.State{
		Holder: flow.RoleOrchestrator, Status: flow.StatusEscalated, Round: 3, Cap: 3,
	}
	ready := flow.State{Holder: flow.RoleOrchestrator, Status: flow.StatusReady, Round: 1, Cap: 3}
	approve, reject := []string{"--verdict", "approve"}, []string{"--verdict", "reject"}

	cases := []struct {
		name  string
		now   flow.State
		role  flow.Role
		args  []string // After --note.
		gated bool
	}{
		{"the lead's brief", running(flow.RoleLead, 0), flow.RoleLead, nil, false},
		{"the engineer's handover", running(flow.RoleEngineer, 1), flow.RoleEngineer, nil, true},
		{"QA's approval", running(flow.RoleQA, 1), flow.RoleQA, approve, true},
		{"QA's rejection", running(flow.RoleQA, 1), flow.RoleQA, reject, false},
		{
			"the reviewer's approval",
			running(flow.RoleReviewer, 1),
			flow.RoleReviewer,
			approve,
			false,
		},
		{
			"the reviewer's rejection",
			running(flow.RoleReviewer, 1),
			flow.RoleReviewer,
			reject,
			false,
		},
		{"the lead's final report", running(flow.RoleLead, 1), flow.RoleLead, nil, true},
		{
			"the orchestrator's new brief", ready, flow.RoleOrchestrator,
			[]string{slug, "--to", "lead"},
			false,
		},
		{
			"the orchestrator's extra round", escalated, flow.RoleOrchestrator,
			[]string{slug, "--to", "engineer"},
			false,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mainDir, linked := repos(t)
			store := started(t, mainDir, test.now)
			note := write(t, t.TempDir(), "note.md", "Done.\n")

			env := paneOf(test.role)
			if test.role == flow.RoleOrchestrator {
				env = orchestratorPane()
			}

			got := run(t, linked, env,
				slices.Concat([]string{"handover", "--note", note}, test.args)...)

			ran, gate := ranGate(mainDir, checked, "checked in main\n"), ""
			if test.gated {
				gate = checked
			}

			if got.code != 0 || got.stderr != "" ||
				strings.HasPrefix(got.stdout, ran) != test.gated ||
				!strings.Contains(got.stdout, "Handed demo-1 over to ") {
				t.Errorf("got %+v; want a handover, after the gate: %t", got, test.gated)
			}

			if logged := lastEvent(t, store).Gate; logged != gate {
				t.Errorf("the handover's event logs gate %q; want %q", logged, gate)
			}
		})
	}
}

// TestHandoverWaitsForTheGateToPass has the engineer hand over while the gate fails: baton shows
// the gate's output, both its streams, and refuses, and the feature stays as it was.
func TestHandoverWaitsForTheGateToPass(t *testing.T) {
	const failing = "echo out; echo err >&2; exit 3"

	withGate(t, failing)

	mainDir, linked := repos(t)
	store := started(t, mainDir, flow.State{
		Holder: flow.RoleEngineer, Status: flow.StatusRunning, Round: 1, Cap: 3,
	})
	before := snapshot(t, store.Dir)
	note := write(t, linked, "note.md", "Done.\n")

	got := run(t, linked, paneOf(flow.RoleEngineer), "handover", "--note", note)

	want := result{
		code:   1,
		stdout: ranGate(mainDir, failing, "out\nerr\n"),
		stderr: `baton handover: gate "` + failing + `" failed: exit status 3: the feature ` +
			"stays with you until it passes\n",
	}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}

	if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
		t.Errorf("baton's folder changed to %v", after)
	}
}

// lastEvent returns the last event in the feature's journal.
func lastEvent(t *testing.T, store state.Store) state.Event {
	t.Helper()

	path := filepath.Join(store.Dir, "features", slug, "events.ndjson")

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")

	var event state.Event
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &event); err != nil {
		t.Fatal(err)
	}

	return event
}

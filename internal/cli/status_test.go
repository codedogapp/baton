package cli_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

func TestStatusListsTheFeatures(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	add(t, store, state.Feature{Slug: "demo-2", Flow: flow.State{
		Holder: flow.RoleOrchestrator, Status: flow.StatusEscalated, Round: 4, Cap: 3, Extra: 1,
	}})

	got := run(t, linked, nil, "status")

	want := result{stdout: "" +
		"FEATURE  HOLDER        ROUND   STATUS     CREDITS  LEAD  ENGINEER  QA  REVIEWER\n" +
		"demo-1   lead          0 of 3  running    -        -     -         -   -\n" +
		"demo-2   orchestrator  4 of 4  escalated  -        -     -         -   -\n"}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

// TestStatusShowsTheAICredits shows the AI credits that each role of a feature has spent, from
// the events of its copilot session in the copilot home folder that COPILOT_HOME names, and their
// sum: a hyphen for a role whose pane started no session, or whose copilot logged no events yet,
// and for the sum when no role's copilot did (decision 66).
func TestStatusShowsTheAICredits(t *testing.T) {
	home := t.TempDir()
	sessions := state.Sessions{Lead: uuid.New(), Engineer: uuid.New(), QA: uuid.New()}
	logEvents(t, home, sessions.Lead, 1_250_000_000, 500_000_000)
	logEvents(t, home, sessions.Engineer)

	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	edit(t, store, func(feature *state.Feature) { feature.Sessions = sessions })
	add(t, store, state.Feature{
		Slug: "demo-2", Flow: fresh(), Sessions: state.Sessions{Reviewer: uuid.New()},
	})

	got := run(t, linked, map[string]string{"COPILOT_HOME": home}, "status")

	want := result{stdout: "" +
		"FEATURE  HOLDER  ROUND   STATUS   CREDITS  LEAD  ENGINEER  QA  REVIEWER\n" +
		"demo-1   lead    0 of 3  running  1.75     1.75  0.00      -   -\n" +
		"demo-2   lead    0 of 3  running  -        -     -         -   -\n"}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

// TestStatusReportsEventsThatItCantRead fails for a role's events that it can't read, and for a
// role's session without copilot's home folder, which neither COPILOT_HOME nor HOME names.
func TestStatusReportsEventsThatItCantRead(t *testing.T) {
	home, session := t.TempDir(), uuid.New()
	events := filepath.Join(home, "session-state", session.String(), "events.jsonl")

	if err := os.MkdirAll(events, 0o700); err != nil {
		t.Fatal(err)
	}

	mainDir, _ := repos(t)
	edit(t, started(t, mainDir, fresh()), func(feature *state.Feature) {
		feature.Sessions.QA = session
	})

	got := run(t, mainDir, map[string]string{"COPILOT_HOME": home}, "status")

	want := "baton status: read the qa's AI credits in demo-1: read " + events + ": is a directory\n"
	if got != (result{code: 1, stderr: want}) {
		t.Errorf("got %+v; want exit code 1 and %q", got, want)
	}

	t.Setenv("HOME", "")

	got = run(t, mainDir, nil, "status")

	want = "baton status: read the qa's AI credits in demo-1: find the home folder: "
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("got %+v; want exit code 1 and %q", got, want)
	}
}

// logEvents writes the events of the copilot session in copilot's home folder: a usage record for
// each model call, with the nano AIU that the call spent (fact 48).
func logEvents(t *testing.T, home string, session uuid.UUID, calls ...int) {
	t.Helper()

	folder := filepath.Join(home, "session-state", session.String())
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}

	var events strings.Builder
	for _, nano := range calls {
		fmt.Fprintf(&events, `{"type":"session.usage_record","data":{"usage":{"copilotUsage":`+
			`{"totalNanoAiu":%d}}}}`+"\n", nano)
	}

	write(t, folder, "events.jsonl", events.String())
}

func TestStatusSaysWhenThereAreNoFeatures(t *testing.T) {
	mainDir, _ := repos(t)

	got := run(t, mainDir, nil, "status")

	if want := (result{stdout: "No features yet.\n"}); got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

func TestStatusRefuses(t *testing.T) {
	mainDir, _ := repos(t)
	broken, _ := repos(t)
	brokenFile := write(t, filepath.Join(started(t, broken, fresh()).Dir, "features", slug),
		"feature.json", `{"format": 2}`)

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	cases := []struct {
		name string
		dir  string
		args []string
		want string // The start of the reason, after "baton status: ".
	}{
		{"with an argument", mainDir, []string{"now"}, "unexpected argument \"now\"\n"},
		{"outside a git repo", outside, nil, "find the git repo: fatal: not a git repository"},
		{
			"with a broken feature", broken, nil,
			"feature file " + brokenFile + ": format 2 is newer than this baton's, 1: " +
				"run the newer baton\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := run(t, test.dir, nil, append([]string{"status"}, test.args...)...)

			want := "baton status: " + test.want
			if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("got %+v; want exit code 1 and %q", got, want)
			}
		})
	}
}

// failing is an output that fails each write.
type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestStatusReportsAnOutputItCantWrite(t *testing.T) {
	empty, _ := repos(t)
	mainDir, _ := repos(t)
	started(t, mainDir, fresh())

	for _, dir := range []string{empty, mainDir} {
		var stderr strings.Builder

		code := cli.Run(t.Context(), cli.Env{
			Args:   []string{"status"},
			Getenv: func(string) string { return "" },
			Dir:    dir,
			Stdout: failing{},
			Stderr: &stderr,
		})

		if got := stderr.String(); code != 1 || got != "baton status: output closed\n" {
			t.Errorf("in %s: exit code %d and %q; want 1 and the output's error", dir, code, got)
		}
	}
}

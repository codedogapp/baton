package cli_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

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
		"FEATURE  HOLDER        ROUND   STATUS\n" +
		"demo-1   lead          0 of 3  running\n" +
		"demo-2   orchestrator  4 of 4  escalated\n"}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
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
	t.Setenv("LC_ALL", "C")

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

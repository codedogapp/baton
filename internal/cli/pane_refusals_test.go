package cli_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestPaneRefusesAndWritesNothing refuses panes that could never start their copilot, or that
// have no feature to wait on, or no work to wait for.
func TestPaneRefusesAndWritesNothing(t *testing.T) {
	_, cache := standIn(t)
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	add(t, store, state.Feature{Slug: "demo-2", Flow: fresh()})

	demo2 := map[string]string{"BATON_ROLE": "lead", "BATON_FEATURE": "demo-2"}
	if got := run(t, mainDir, demo2, "wait"); got.code != 0 ||
		!strings.HasPrefix(got.stdout, "Picked up demo-2 as lead: ") {
		t.Fatalf("the lead's wait in demo-2: %+v", got)
	}

	before := snapshot(t, store.Dir)
	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	cases := []struct {
		name   string
		setup  func(t *testing.T)
		dir    string
		env    map[string]string
		args   []string
		self   error // What finding baton's own program fails with, if set.
		stdout string
		want   string // The start of the reason, after "baton pane: ".
	}{
		{
			name: "with an argument",
			dir:  linked,
			env:  paneOf(flow.RoleLead),
			args: []string{"now"},
			want: `unexpected argument "now"`,
		},
		{
			name: "outside a pane", dir: linked, env: nil,
			want: "want BATON_ROLE: run it in a pane that baton started, which names its role " +
				"there\n",
		},
		{
			name: "without its feature", dir: linked, env: map[string]string{"BATON_ROLE": "lead"},
			want: "want BATON_FEATURE: the lead's pane names its feature there\n",
		},
		{
			name: "without its own program", dir: linked, env: paneOf(flow.RoleLead),
			self: errors.New("no such file"),
			want: "find baton's own program: no such file\n",
		},
		{
			name: "without copilot", dir: linked, env: paneOf(flow.RoleLead),
			setup: func(t *testing.T) { t.Setenv("PATH", t.TempDir()) },
			want:  `exec: "copilot": executable file not found in $PATH` + "\n",
		},
		{
			name: "the orchestrator's, without copilot", dir: mainDir, env: orchestratorPane(),
			setup: func(t *testing.T) { t.Setenv("PATH", t.TempDir()) },
			want:  `exec: "copilot": executable file not found in $PATH` + "\n",
		},
		{
			name: "the orchestrator's, with a feature", dir: mainDir,
			env:  map[string]string{"BATON_ROLE": "orchestrator", "BATON_FEATURE": "demo-1"},
			want: "the orchestrator's pane names no feature, but BATON_FEATURE names demo-1\n",
		},
		{
			name: "without a home folder", dir: linked, env: paneOf(flow.RoleLead),
			setup: func(t *testing.T) { t.Setenv("HOME", "") },
			want:  "find the cache folder: ",
		},
		{
			name: "with a file for its cache folder", dir: linked, env: paneOf(flow.RoleLead),
			setup: func(t *testing.T) { fileFor(t, cache) },
			want:  "make the cache folder: mkdir " + cache + ": not a directory\n",
		},
		{
			name: "outside a git repo", dir: outside, env: paneOf(flow.RoleLead),
			want: "find the git repo: fatal: not a git repository",
		},
		{
			name: "of another feature", dir: linked,
			env:    map[string]string{"BATON_ROLE": "lead", "BATON_FEATURE": "demo-3"},
			stdout: "lead in demo-3: waiting for work.\n",
			want:   "no feature demo-3\n",
		},
		{
			name:   "of a feature whose work is picked up",
			dir:    linked,
			env:    demo2,
			stdout: "lead in demo-2: waiting for work.\n",
			want:   "lead holds demo-2, with no work left to pick up: hand it over before waiting\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			synctest.Test(t, func(t *testing.T) {
				env, _ := inPane(t, test.dir, test.env)
				env.Args = append(env.Args, test.args...)

				if test.self != nil {
					env.Executable = func() (string, error) { return "", test.self }
				}

				wantRefused(t, paneFor(t, env), test.stdout, test.want)
			})

			if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed to %v", after)
			}
		})
	}
}

// fileFor puts a file where the folder goes, until the test ends.
func fileFor(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Dir(dir), filepath.Base(dir), "")
	t.Cleanup(func() { _ = os.Remove(dir) })
}

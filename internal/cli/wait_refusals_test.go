package cli_test

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestWaitStopsWhenItsContextEnds stops a wait for work that never comes.
func TestWaitStopsWhenItsContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())
		before := snapshot(t, store.Dir)

		ctx, stop := context.WithCancelCause(t.Context())
		waited := make(chan result, 1)

		go func() { waited <- runUntil(ctx, linked, paneOf(flow.RoleQA), "wait") }()

		time.Sleep(time.Minute)
		stop(errors.New("interrupt signal received"))

		want := result{
			code:   1,
			stderr: "baton wait: stopped waiting: interrupt signal received\n",
		}
		if got := <-waited; got != want {
			t.Errorf("got %+v; want %+v", got, want)
		}

		if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
			t.Errorf("baton's folder changed to %v", after)
		}
	})
}

// TestWaitRefusesAndWritesNothing refuses waits that could never end, or that have no feature to
// wait on.
func TestWaitRefusesAndWritesNothing(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	add(t, store, state.Feature{Slug: "demo-2", Flow: fresh()})

	// demo-2's file loses the lead's work, as a hand edit could.
	edit := state.Entry{Event: state.Event{Kind: "edit", Role: flow.RoleOrchestrator}}
	if _, err := store.Update("demo-2", edit, func(feature *state.Feature) error {
		feature.Work = state.Work{}

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	before := snapshot(t, store.Dir)
	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	cases := []struct {
		name string
		dir  string
		env  map[string]string
		args []string
		want string // The start of the reason, after "baton wait: ".
	}{
		{
			"with an argument",
			linked,
			paneOf(flow.RoleLead),
			[]string{"now"},
			`unexpected argument "now"`,
		},
		{
			"outside a pane", linked,
			map[string]string{"BATON_ROLE": "lead"},
			nil,
			"run it in a role's pane, where BATON_ROLE and BATON_FEATURE name the role and " +
				"the feature\n",
		},
		{
			"with an unknown role", linked, paneOf("boss"), nil,
			`unknown role "boss" in BATON_ROLE` + "\n",
		},
		{
			"of a feature that lost its work", linked,
			map[string]string{"BATON_ROLE": "lead", "BATON_FEATURE": "demo-2"},
			nil,
			"lead holds demo-2, with no work left to pick up: hand it over before waiting\n",
		},
		{
			"of another feature", linked,
			map[string]string{"BATON_ROLE": "lead", "BATON_FEATURE": "demo-3"},
			nil,
			"no feature demo-3\n",
		},
		{
			"outside a git repo", outside, paneOf(flow.RoleLead), nil,
			"find the git repo: fatal: not a git repository",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				got := runFor(t, test.dir, test.env, append([]string{"wait"}, test.args...)...)

				want := "baton wait: " + test.want
				if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
					t.Errorf("got %+v; want exit code 1 and %q", got, want)
				}
			})

			if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed to %v", after)
			}
		})
	}
}

// TestWaitReportsAnOutputItCantWrite fails a wait that can't print the work it picked up. The
// pickup stays recorded, so no other wait gets the work too.
func TestWaitReportsAnOutputItCantWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, fresh())

		var stderr strings.Builder

		code := cli.Run(t.Context(), cli.Env{
			Args:   []string{"wait"},
			Getenv: func(name string) string { return paneOf(flow.RoleLead)[name] },
			Dir:    mainDir,
			Stdout: failing{},
			Stderr: &stderr,
		})

		if got := stderr.String(); code != 1 || got != "baton wait: output closed\n" {
			t.Errorf("exit code %d and %q; want 1 and the output's error", code, got)
		}

		wantWork(t, store, state.Work{
			Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator, Picked: true,
		})
	})
}

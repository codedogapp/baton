package cli_test

import (
	"context"
	"errors"
	"io"
	"maps"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestPaneStopsWaiting stops panes that wait for work that never comes: the terminal's input
// ends, as when the user presses Ctrl-D, or the terminal fails, as when it's gone (fact 22), or
// baton's context ends, as when the user presses Ctrl-C.
func TestPaneStopsWaiting(t *testing.T) {
	standIn(t)

	cases := []struct {
		name string
		stop func(user *io.PipeWriter, cancel context.CancelCauseFunc)
		want string // The reason, after "baton pane: stopped waiting: ".
	}{
		{
			"when the terminal's input ends",
			func(user *io.PipeWriter, _ context.CancelCauseFunc) { _ = user.Close() },
			"the terminal's input ended: EOF\n",
		},
		{
			"when the terminal fails",
			func(user *io.PipeWriter, _ context.CancelCauseFunc) {
				_ = user.CloseWithError(errors.New("read /dev/ttys001: input/output error"))
			},
			"the terminal's input ended: read /dev/ttys001: input/output error\n",
		},
		{
			"when its context ends",
			func(_ *io.PipeWriter, cancel context.CancelCauseFunc) {
				cancel(errors.New("interrupt signal received"))
			},
			"interrupt signal received\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mainDir, linked := repos(t)
				store := started(t, mainDir, fresh())
				before := snapshot(t, store.Dir)

				env, user := inPane(t, linked, paneOf(flow.RoleQA))
				ctx, cancel := context.WithCancelCause(t.Context())

				defer cancel(nil)

				// A pane that misses its end stops after an hour of the bubble's time.
				ctx, stop := context.WithTimeout(ctx, time.Hour)
				defer stop()

				waited := make(chan ran, 1)
				go func() { waited <- runExec(ctx, env, nil) }()

				time.Sleep(time.Minute)
				test.stop(user, cancel)

				wantRefused(t, <-waited, "qa in demo-1: waiting for work.\n",
					"stopped waiting: "+test.want)

				if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
					t.Errorf("baton's folder changed to %v", after)
				}
			})
		})
	}
}

// TestPaneReportsACopilotThatFailsToStart fails a pane whose copilot doesn't start. The pickup
// stays recorded, so no other pane gets the work too.
func TestPaneReportsACopilotThatFailsToStart(t *testing.T) {
	standIn(t)

	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, fresh())

		env, _ := inPane(t, mainDir, paneOf(flow.RoleLead))
		got := runExec(t.Context(), env, errors.New("exec format error"))

		want := result{
			code:   1,
			stdout: "lead in demo-1: waiting for work.\n",
			stderr: "baton pane: start copilot: exec format error\n",
		}
		if got.result != want || len(got.execs) != 1 {
			t.Errorf("got %+v; want %+v, after a command line", got, want)
		}

		wantWork(t, store, state.Work{
			Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator, Picked: true,
		})
	})
}

// TestPaneReportsAnOutputItCantWrite fails a pane that can't say that it waits, before it picks
// up any work.
func TestPaneReportsAnOutputItCantWrite(t *testing.T) {
	standIn(t)

	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, fresh())

		var stderr strings.Builder

		env, _ := inPane(t, mainDir, paneOf(flow.RoleLead))
		env.Stdout, env.Stderr = failing{}, &stderr
		env.Exec = func(string, []string, []string) error {
			t.Error("Exec ran")

			return nil
		}

		if code, got := cli.Run(t.Context(), env), stderr.String(); code != 1 ||
			got != "baton pane: output closed\n" {
			t.Errorf("exit code %d and %q; want 1 and the output's error", code, got)
		}

		wantWork(
			t,
			store,
			state.Work{Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator},
		)
	})
}

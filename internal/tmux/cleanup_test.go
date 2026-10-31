package tmux_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/tmux"
)

// TestNewSessionEndsASessionThatItCannotFill asks for more panes than a session has room for.
func TestNewSessionEndsASessionThatItCannotFill(t *testing.T) {
	srv := server(t, "set-option -g default-size 4x3")
	dir := t.TempDir()

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: "keep", Dir: dir, Panes: []tmux.Pane{pane("lead")},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = srv.NewSession(t.Context(), tmux.Session{
		Name: "full", Dir: dir, Panes: []tmux.Pane{pane("lead"), pane("engineer"), pane("qa")},
	})

	// tmux 3.7c puts "size or position" before the reason (fact 22).
	want := "make the tmux session full: tmux split-window: no space for a new pane"
	if err == nil || strings.Replace(err.Error(), "size or position ", "", 1) != want {
		t.Errorf("NewSession = %v; want %s", err, want)
	}

	if sessions := on(t, srv, "list-sessions", "-F", "#{session_name}"); sessions != "keep" {
		t.Errorf("the sessions are %q; want keep", sessions)
	}
}

// TestNewSessionEndsNoOtherSession has a hook end the session while NewSession fills it, next to a
// session whose name starts with the session's name.
func TestNewSessionEndsNoOtherSession(t *testing.T) {
	for _, test := range []struct{ hook, failure string }{
		{"after-new-session", "set-option: no such window: @1"},
		{"after-split-window", "split-window: can't find window: @1"},
		{"after-select-layout", "select-pane: can't find pane: %1"},
	} {
		t.Run(test.hook, func(t *testing.T) {
			srv := server(t)
			dir := t.TempDir()

			err := srv.NewSession(t.Context(), tmux.Session{
				Name: "demo-2", Dir: dir, Panes: []tmux.Pane{pane("lead")},
			})
			if err != nil {
				t.Fatal(err)
			}

			on(t, srv, "set-hook", "-g", test.hook, "kill-session")

			err = srv.NewSession(t.Context(), tmux.Session{
				Name: "demo", Dir: dir, Panes: []tmux.Pane{pane("lead"), pane("engineer")},
			})

			want := "make the tmux session demo: tmux " + test.failure +
				"\ntmux kill-session: can't find session: demo"
			if err == nil || err.Error() != want {
				t.Errorf("NewSession = %v; want %s", err, want)
			}

			sessions := on(t, srv, "list-sessions", "-F", "#{session_name}")
			if sessions != "demo-2" {
				t.Errorf("the sessions are %q; want demo-2", sessions)
			}
		})
	}
}

// TestNewSessionEndsTheSessionWhenAHookFails has a hook of the user's fail once tmux has made the
// session, next to a session whose name starts with the session's name.
func TestNewSessionEndsTheSessionWhenAHookFails(t *testing.T) {
	srv := server(t)
	dir := t.TempDir()

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: "demo-2", Dir: dir, Panes: []tmux.Pane{pane("lead")},
	})
	if err != nil {
		t.Fatal(err)
	}

	on(t, srv, "set-hook", "-g", "after-new-session", "select-pane -t nosuch")

	err = srv.NewSession(t.Context(), tmux.Session{
		Name: "demo", Dir: dir, Panes: []tmux.Pane{pane("lead")},
	})
	if want := "make the tmux session demo: tmux new-session: can't find pane: nosuch"; err == nil ||
		err.Error() != want {
		t.Errorf("NewSession = %v; want %s", err, want)
	}

	if sessions := on(t, srv, "list-sessions", "-F", "#{session_name}"); sessions != "demo-2" {
		t.Errorf("the sessions are %q; want demo-2", sessions)
	}
}

// TestNewSessionEndsTheSessionWhenStopped stops NewSession while a hook holds up its split.
func TestNewSessionEndsTheSessionWhenStopped(t *testing.T) {
	// wait-for holds the split until the server ends, which a job of run-shell's would put off.
	srv := server(t, "set-hook -g after-split-window 'wait-for baton-test'")
	errStop := errors.New("stopped")
	ctx, stop := context.WithCancelCause(t.Context())

	var split sync.WaitGroup

	split.Go(func() {
		// Once the split is in, NewSession waits for the hook.
		for ctx.Err() == nil {
			out, _ := command(ctx, srv, "list-panes", "-s", "-t", "=slow", "-F", "#{pane_id}").
				Output()
			if strings.Count(string(out), "%") == 2 {
				stop(errStop)
			}

			time.Sleep(10 * time.Millisecond)
		}
	})

	err := srv.NewSession(ctx, tmux.Session{
		Name: "slow", Dir: t.TempDir(), Panes: []tmux.Pane{pane("lead"), pane("engineer")},
	})

	stop(nil)
	split.Wait()

	if want := "make the tmux session slow: tmux split-window: stopped"; !errors.Is(err, errStop) ||
		err.Error() != want {
		t.Errorf("NewSession = %v; want %s", err, want)
	}

	if err := command(t.Context(), srv, "has-session", "-t", "=slow").Run(); err == nil {
		t.Error("the session is still there; want it ended")
	}
}

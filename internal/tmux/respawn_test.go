package tmux_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/tmux"
)

// TestNewSessionKeepsThePanesThatEnd makes a session that keeps its panes, whose programs end at
// once, the first one before tmux would run another call, and checks that tmux keeps them, dead,
// with what they printed. The option is the window's alone.
func TestNewSessionKeepsThePanesThatEnd(t *testing.T) {
	srv := server(t)
	dir, shown := folder(t)
	ends := []string{"sh", "-c", `printf '%s|%s\n' "$(pwd -P)" "${BATON_TMUX_TEST-unset}"; exit 3`}

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: "demo", Dir: dir, Remain: true, Panes: []tmux.Pane{
			{Title: "lead", Env: []string{"BATON_TMUX_TEST=lead"}, Command: ends},
			{Title: "engineer", Command: ends},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	lists(t, srv, "lead 1 3\nengineer 1 3",
		"list-panes", "-s", "-t", "=demo", "-F", "#{pane_title} #{pane_dead} #{pane_dead_status}")

	ids := panes(t, srv, "demo")
	shows(t, srv, ids["lead"], shown+"|lead")
	shows(t, srv, ids["engineer"], shown+"|unset")

	window := on(t, srv, "show-options", "-w", "-v", "-t", "=demo:", "remain-on-exit")
	global := on(t, srv, "show-options", "-g", "-w", "-v", "remain-on-exit")

	if window != "on" || global != "off" {
		t.Errorf("remain-on-exit is %q in the window and %q for the server; want on and off",
			window, global)
	}
}

// TestRespawnStartsTheDeadPanesAgain ends two of a session's three panes, and a pane of the
// user's, then starts the session's dead panes again, with other environments, in another folder.
func TestRespawnStartsTheDeadPanesAgain(t *testing.T) {
	srv := server(t)
	dir, shown := folder(t)
	session := tmux.Session{Name: "demo", Dir: dir, Remain: true, Panes: []tmux.Pane{
		pane("lead", "BATON_TMUX_TEST=lead"), pane("engineer"), pane("qa", "BATON_TMUX_TEST=qa"),
	}}

	if err := srv.NewSession(t.Context(), session); err != nil {
		t.Fatal(err)
	}

	ids := panes(t, srv, "demo")
	user := on(t, srv, "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", ids["qa"], "-c", dir,
		"sh", "-c", role)

	for id, line := range map[string]string{
		ids["lead"]: shown + "|lead", ids["qa"]: shown + "|qa", user: shown + "|unset",
	} {
		shows(t, srv, id, line)
		on(t, srv, "send-keys", "-t", id, "Enter")
	}

	listing := []string{"list-panes", "-s", "-t", "=demo", "-F", "#{pane_id} #{pane_dead}"}
	lists(t, srv, ids["lead"]+" 1\n"+ids["engineer"]+" 0\n"+ids["qa"]+" 1\n"+user+" 1", listing...)

	session.Dir, shown = folder(t)
	session.Panes = []tmux.Pane{pane("lead", "BATON_TMUX_TEST=again"), pane("engineer"), pane("qa")}

	started, err := srv.Respawn(t.Context(), session)
	if err != nil || !slices.Equal(started, []string{"lead", "qa"}) {
		t.Fatalf("Respawn = %q, %v; want lead and qa", started, err)
	}

	shows(t, srv, ids["lead"], shown+"|again")
	shows(t, srv, ids["qa"], shown+"|unset")
	lists(t, srv, ids["lead"]+" 0\n"+ids["engineer"]+" 0\n"+ids["qa"]+" 0\n"+user+" 1", listing...)

	titles := on(t, srv, "list-panes", "-s", "-t", "=demo", "-F", "#{pane_title}")
	if want := "lead\nengineer\nqa\n"; !strings.HasPrefix(titles, want) {
		t.Errorf("the panes are titled %q; want %q first", titles, want)
	}

	if started, err := srv.Respawn(t.Context(), session); err != nil || started != nil {
		t.Errorf("Respawn again = %q, %v; want nothing started", started, err)
	}
}

// TestRespawnRefuses starts again the dead panes of sessions where it can't: one that isn't there,
// one whose folder is gone, and one that loses a dead pane to a hook of the user's right after
// tmux lists them, where it starts the pane before that one.
func TestRespawnRefuses(t *testing.T) {
	srv := server(t, "set-hook -g after-list-panes 'kill-pane -t =demo:0.1'")
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	ends := []string{"sh", "-c", "exit 3"}
	session := tmux.Session{Name: "demo", Dir: dir, Remain: true, Panes: []tmux.Pane{
		{Title: "lead", Command: ends}, {Title: "qa", Command: ends},
	}}

	if err := srv.NewSession(t.Context(), session); err != nil {
		t.Fatal(err)
	}

	// display-message, unlike list-panes, runs no hook of the user's.
	for _, target := range []string{"=demo:0.0", "=demo:0.1"} {
		lists(t, srv, "1", "display-message", "-p", "-t", target, "#{pane_dead}")
	}

	moved := session
	moved.Dir = missing

	// In this order, since only the last one lists the panes, which runs the hook.
	for _, test := range []struct {
		name    string
		session tmux.Session
		started []string
		reason  string // What the reason starts with.
	}{
		{
			name:    "a session that isn't there",
			session: tmux.Session{Name: "nope", Dir: dir, Panes: session.Panes},
			reason:  "find the dead panes of tmux session nope: tmux list-panes: ",
		},
		{
			name:    "a folder that's gone",
			session: moved,
			reason:  "tmux session demo: stat " + missing + ": no such file or directory",
		},
		{
			name:    "a session that loses a pane",
			session: session,
			started: []string{"lead"},
			reason:  "start the qa pane of tmux session demo again: tmux respawn-pane: ",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			started, err := srv.Respawn(t.Context(), test.session)
			if !slices.Equal(started, test.started) || err == nil ||
				!strings.HasPrefix(err.Error(), test.reason) {
				t.Errorf("Respawn = %q, %v; want %q, and a reason that starts %q", started, err,
					test.started, test.reason)
			}
		})
	}
}

// lists waits until tmux prints the text for the arguments, for up to ten seconds.
func lists(t *testing.T, srv tmux.Server, want string, args ...string) {
	t.Helper()

	var got string

	for range 1000 {
		if got = on(t, srv, args...); got == want {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("tmux %s printed %q; want %q", strings.Join(args, " "), got, want)
}

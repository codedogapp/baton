package tmux_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/tmux"
)

// TestSessionDirFindsTheSessionByItsWholeName looks for a session before its server runs, while
// only a session whose name starts with its name runs, and once it runs, with two windows, in a
// folder whose name holds a #, which tmux reads in -c as a format's start (fact 22).
func TestSessionDirFindsTheSessionByItsWholeName(t *testing.T) {
	srv := server(t)

	if dir, found := srv.SessionDir(t.Context(), "baton-repo"); found {
		t.Errorf("SessionDir before the server runs = %q, true; want false", dir)
	}

	feature, _ := folder(t)
	making(t, srv, "baton-repo-demo-1", feature)

	if dir, found := srv.SessionDir(t.Context(), "baton-repo"); found {
		t.Errorf("SessionDir with baton-repo-demo-1 alone = %q, true; want false", dir)
	}

	repo := filepath.Join(t.TempDir(), "my#repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}

	making(t, srv, "baton-repo", repo)
	on(t, srv, "new-window", "-d", "-t", "=baton-repo:", "sh", "-c", role)

	if dir, found := srv.SessionDir(t.Context(), "baton-repo"); dir != repo || !found {
		t.Errorf("SessionDir = %q, %v; want %q, true", dir, found, repo)
	}
}

// TestSwitchClientShowsTheSession attaches a client to a session, as a user's terminal, then
// switches it to another session from the pane that it shows, as baton start does inside tmux.
func TestSwitchClientShowsTheSession(t *testing.T) {
	srv := server(t)
	dir, _ := folder(t)

	making(t, srv, "first", dir)
	making(t, srv, "second", dir)
	attach(t, srv, "first")
	t.Setenv("TMUX_PANE", panes(t, srv, "first")["first"])

	if err := srv.SwitchClient(t.Context(), "second"); err != nil {
		t.Fatal(err)
	}

	if shown := on(t, srv, "list-clients", "-F", "#{client_session}"); shown != "second" {
		t.Errorf("the client shows %q; want second", shown)
	}

	err := srv.SwitchClient(t.Context(), "sec")
	if want := "tmux switch-client: can't find session: sec"; err == nil || err.Error() != want {
		t.Errorf("SwitchClient to a session that isn't there: %v; want %s", err, want)
	}
}

// TestAttachCommandNamesTheServer checks the command line that attaches a terminal to a session,
// on the user's server and on one of baton's own.
func TestAttachCommandNamesTheServer(t *testing.T) {
	for _, test := range []struct {
		srv  tmux.Server
		want []string
	}{
		{tmux.Server{}, []string{"tmux", "attach-session", "-t", "=baton-repo"}},
		{tmux.Server{Socket: "baton-live", Config: os.DevNull}, []string{
			"tmux", "-L", "baton-live", "-f", os.DevNull, "attach-session", "-t", "=baton-repo",
		}},
	} {
		if got := test.srv.AttachCommand("baton-repo"); !slices.Equal(got, test.want) {
			t.Errorf("%+v.AttachCommand = %q; want %q", test.srv, got, test.want)
		}
	}
}

// making makes a session with one pane, titled with the session's name, in the folder.
func making(t *testing.T, srv tmux.Server, name, dir string) {
	t.Helper()

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: name, Dir: dir, Panes: []tmux.Pane{pane(name)},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// attach attaches a client to the session until the test ends, in control mode, which needs no
// terminal, and waits until tmux lists it.
func attach(t *testing.T, srv tmux.Server, session string) {
	t.Helper()

	client := command(t.Context(), srv, "-C", "attach-session", "-t", "="+session)

	// The client stays while its input is open, and the test's end kills it.
	if _, err := client.StdinPipe(); err != nil {
		t.Fatal(err)
	}

	if err := client.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = client.Wait() })

	for range 1000 {
		if on(t, srv, "list-clients", "-F", "#{client_session}") == session {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("tmux lists no client of %s", session)
}

package tmux_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/tmux"
)

// TestNewSessionRefusesWhatTmuxWouldMakeWrong asks for sessions that tmux would rename, find by
// mistake or start in another folder, and for a session without panes.
func TestNewSessionRefusesWhatTmuxWouldMakeWrong(t *testing.T) {
	srv := server(t)
	dir, one := t.TempDir(), []tmux.Pane{pane("lead")}
	file, missing := filepath.Join(dir, "file"), filepath.Join(dir, "missing")

	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", ".", "a.b", "a:b", "a b", "=a", "#a", "dé", "a\n"} {
		err := srv.NewSession(t.Context(), tmux.Session{Name: name, Dir: dir, Panes: one})

		want := fmt.Sprintf("tmux session name %q: want letters, digits, hyphens and underscores",
			name)
		if err == nil || err.Error() != want {
			t.Errorf("NewSession(%q) = %v; want %s", name, err, want)
		}
	}

	for _, test := range []struct {
		dir   string
		panes []tmux.Pane
		want  string
	}{
		{dir, nil, "tmux session demo: want a pane"},
		{"docs", one, `tmux session demo: want an absolute folder, got "docs"`},
		{missing, one, "tmux session demo: stat " + missing + ": no such file or directory"},
		{file, one, "tmux session demo: " + file + " is not a folder"},
	} {
		err := srv.NewSession(
			t.Context(),
			tmux.Session{Name: "demo", Dir: test.dir, Panes: test.panes},
		)
		if err == nil || err.Error() != test.want {
			t.Errorf("NewSession in %s with %d panes = %v; want %s", test.dir, len(test.panes), err,
				test.want)
		}
	}

	err := srv.NewSession(t.Context(), tmux.Session{Name: "demo", Dir: missing, Panes: one})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("NewSession in %s = %v; want an error that is fs.ErrNotExist", missing, err)
	}

	if err := command(t.Context(), srv, "list-sessions").Run(); err == nil {
		t.Error("tmux started a server; want none")
	}
}

// TestNewSessionLeavesAnExistingSessionAlone asks for a session that tmux already has.
func TestNewSessionLeavesAnExistingSessionAlone(t *testing.T) {
	srv := server(t)
	session := tmux.Session{
		Name: "demo", Dir: t.TempDir(), Panes: []tmux.Pane{pane("lead"), pane("engineer")},
	}

	if err := srv.NewSession(t.Context(), session); err != nil {
		t.Fatal(err)
	}

	err := srv.NewSession(t.Context(), session)
	if want := "make the tmux session demo: tmux new-session: duplicate session: demo"; err == nil ||
		err.Error() != want {
		t.Errorf("NewSession = %v; want %s", err, want)
	}

	if ids := panes(t, srv, "demo"); len(ids) != len(session.Panes) {
		t.Errorf("the session's panes are %v; want lead and engineer", ids)
	}
}

// TestNewSessionReportsAMissingTmux runs NewSession where no tmux is on the PATH.
func TestNewSessionReportsAMissingTmux(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	srv := tmux.Server{Socket: "baton-test-none"}

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: "demo", Dir: t.TempDir(), Panes: []tmux.Pane{pane("lead")},
	})

	want := "make the tmux session demo: tmux new-session: "
	if !errors.Is(err, exec.ErrNotFound) || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("NewSession = %v; want %s and tmux not found", err, want)
	}
}

// TestNewSessionReportsATmuxThatFailsWithoutAReason runs NewSession with a tmux that only exits 3.
func TestNewSessionReportsATmuxThatFailsWithoutAReason(t *testing.T) {
	bin, executable := t.TempDir(), os.FileMode(0o700)

	err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 3\n"), executable)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin)

	srv := tmux.Server{Socket: "baton-test-none"}

	err = srv.NewSession(t.Context(), tmux.Session{
		Name: "demo", Dir: t.TempDir(), Panes: []tmux.Pane{pane("lead")},
	})
	if want := "make the tmux session demo: tmux new-session: exit status 3"; err == nil ||
		err.Error() != want {
		t.Errorf("NewSession = %v; want %s", err, want)
	}
}

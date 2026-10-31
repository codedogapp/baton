package tmux_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/tmux"
)

// TestNewSessionTilesTitledPanes makes a session for the four roles, as baton names them.
func TestNewSessionTilesTitledPanes(t *testing.T) {
	srv := server(t)
	dir, shown := folder(t)

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: "baton-Repo_2-demo-1", Dir: dir,
		Panes: []tmux.Pane{pane("lead"), pane("engineer"), pane("qa"), pane("reviewer")},
	})
	if err != nil {
		t.Fatal(err)
	}

	layout := on(t, srv, "list-panes", "-s", "-t", "=baton-Repo_2-demo-1", "-F",
		"#{pane_title} #{pane_left},#{pane_top} #{pane_active}")
	if want := "lead 0,1 1\nengineer 40,1 0\nqa 0,12 0\nreviewer 40,12 0"; layout != want {
		t.Errorf("the panes are\n%s\nwant\n%s", layout, want)
	}

	for _, id := range panes(t, srv, "baton-Repo_2-demo-1") {
		shows(t, srv, id, shown+"|unset")
	}

	for _, option := range []struct{ name, window, global string }{
		{"pane-border-status", "top", "off"},
		{"allow-set-title", "off", "on"},
	} {
		window := on(t, srv, "show-options", "-w", "-v", "-t", "=baton-Repo_2-demo-1:",
			option.name)
		global := on(t, srv, "show-options", "-g", "-w", "-v", option.name)

		if window != option.window || global != option.global {
			t.Errorf("%s is %q in the window and %q for the server; want %q and %q",
				option.name, window, global, option.window, option.global)
		}
	}
}

// TestNewSessionKeepsAPanesEnvironmentToThePane gives two of three panes a BATON_TMUX_TEST of
// their own, then splits a pane of the user's off the first.
func TestNewSessionKeepsAPanesEnvironmentToThePane(t *testing.T) {
	srv := server(t)
	dir, shown := folder(t)

	err := srv.NewSession(t.Context(), tmux.Session{Name: "demo", Dir: dir, Panes: []tmux.Pane{
		pane("lead", "BATON_TMUX_TEST=lead"), pane("engineer"), pane("qa", "BATON_TMUX_TEST=qa"),
	}})
	if err != nil {
		t.Fatal(err)
	}

	ids := panes(t, srv, "demo")
	user := on(t, srv, "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", ids["lead"], "-c", dir,
		"sh", "-c", role)

	for id, want := range map[string]string{
		ids["lead"]: "lead", ids["engineer"]: "unset", ids["qa"]: "qa", user: "unset",
	} {
		shows(t, srv, id, shown+"|"+want)
	}
}

// TestNewSessionKeepsTheTitles has a pane's program try to retitle its pane.
func TestNewSessionKeepsTheTitles(t *testing.T) {
	srv := server(t)
	retitle := `read -r _; printf '\033]2;other\033\\'; echo retitled; read -r _`

	err := srv.NewSession(
		t.Context(),
		tmux.Session{Name: "demo", Dir: t.TempDir(), Panes: []tmux.Pane{
			{Title: "lead", Command: []string{"sh", "-c", retitle}},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	id := panes(t, srv, "demo")["lead"]
	on(t, srv, "send-keys", "-t", id, "go", "Enter")
	shows(t, srv, id, "retitled")

	if title := on(t, srv, "display-message", "-p", "-t", id, "#{pane_title}"); title != "lead" {
		t.Errorf("the pane's title is %q; want lead", title)
	}
}

// TestNewSessionTakesTextAsItIs gives two panes text that tmux would otherwise expand or cut short.
func TestNewSessionTakesTextAsItIs(t *testing.T) {
	srv := server(t)
	dir, shown := folder(t)
	dir, shown = filepath.Join(dir, "a #S;"), filepath.Join(shown, "a #S;")

	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	show := `printf '%s|%s|%s\n' "$(pwd -P)" "$BATON_TMUX_TEST" "$0"; read -r _`
	first := tmux.Pane{
		Title: "#S #(true);", Env: []string{"BATON_TMUX_TEST=#{b};"},
		Command: []string{"sh", "-c", show, "#{c};"},
	}
	second := first
	second.Title = "#{session_name};"

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: "demo", Dir: dir, Panes: []tmux.Pane{first, second},
	})
	if err != nil {
		t.Fatal(err)
	}

	ids := panes(t, srv, "demo")
	for _, title := range []string{first.Title, second.Title} {
		id, ok := ids[title]
		if !ok {
			t.Fatalf("no pane is titled %q; the titles and panes are %v", title, ids)
		}

		shows(t, srv, id, shown+"|#{b};|#{c};")
	}
}

// TestNewSessionReadsPastWhatHooksPrint makes a session on a server whose hooks print after the
// commands that make it.
func TestNewSessionReadsPastWhatHooksPrint(t *testing.T) {
	srv := server(t,
		"set-hook -g after-new-session 'display-message -p new-session'",
		"set-hook -g after-split-window 'display-message -p split-window'",
		"set-hook -g after-select-layout 'display-message -p select-layout'",
	)

	err := srv.NewSession(t.Context(), tmux.Session{
		Name: "demo", Dir: t.TempDir(),
		Panes: []tmux.Pane{pane("lead"), pane("engineer"), pane("qa")},
	})
	if err != nil {
		t.Fatal(err)
	}

	titles := on(t, srv, "list-panes", "-s", "-t", "=demo", "-F", "#{pane_title}")
	if want := "lead\nengineer\nqa"; titles != want {
		t.Errorf("the panes are %q; want %q", titles, want)
	}
}

package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/state"
)

// skeleton makes the skeleton test's world, and returns its folder. The folder holds a bin folder,
// first on the PATH, with baton, a link to this test binary, and copilot, a link to the stand-in;
// a home folder and a temp folder of the test's own; the repo, with a commit, which is the test's
// working folder; and the orchestrator's brief. baton's tmux finds the test's own server, as tmux's
// default server, in a TMUX_TMPDIR of the test's own.
func skeleton(t *testing.T) string {
	t.Helper()

	standIn, err := filepath.Abs(filepath.Join("testdata", "copilot"))
	if err != nil {
		t.Fatal(err)
	}

	// The stat has go test's cache watch the stand-in, which the panes run, not the test.
	if _, err := os.Stat(standIn); err != nil {
		t.Fatal(err)
	}

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	root := folder(t, "", "baton-test-skeleton-")
	bin, home, temp, repo := filepath.Join(root, "bin"), filepath.Join(root, "home"),
		filepath.Join(root, "tmp"), filepath.Join(root, "repo")

	for _, dir := range []string{bin, home, temp, repo} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	link(t, self, filepath.Join(bin, "baton"))
	link(t, standIn, filepath.Join(bin, "copilot"))

	err = os.WriteFile(filepath.Join(root, "brief.md"), []byte("Add demo.txt.\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	t.Setenv("TMPDIR", temp)
	t.Setenv("SKELETON_LOG", filepath.Join(root, "copilot.log"))
	t.Setenv(asBaton, "1")
	// macOS limits a socket's path to 104 bytes, and $TMPDIR there leaves too few of them.
	t.Setenv("TMUX_TMPDIR", folder(t, "/tmp", "baton-test-skeleton-"))
	t.Chdir(repo)

	serve(t)
	ran(t, exec.CommandContext(t.Context(), "git", "init", "--quiet", "--initial-branch=main"))
	ran(t, exec.CommandContext(t.Context(), "git", "commit", "--quiet", "--allow-empty",
		"--message", "Start"))

	return root
}

// folder makes a new folder in the folder dir, and returns it, with symlinks resolved. It removes
// it when the test ends, and tries again for a while: the panes' baton, which the end of tmux
// stops, may look at the feature for a moment more.
func folder(t *testing.T, dir, pattern string) string {
	t.Helper()

	made, err := os.MkdirTemp(dir, pattern)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		var err error

		for range 50 {
			if err = os.RemoveAll(made); err == nil {
				return
			}

			time.Sleep(100 * time.Millisecond)
		}

		t.Error(err)
	})

	resolved, err := filepath.EvalSymlinks(made)
	if err != nil {
		t.Fatal(err)
	}

	return resolved
}

// link makes a symlink to the file at the path.
func link(t *testing.T, file, path string) {
	t.Helper()

	if err := os.Symlink(file, path); err != nil {
		t.Fatal(err)
	}
}

// serve starts the test's tmux server, without a config file: tmux's default server in the test's
// TMUX_TMPDIR, which baton's tmux finds. Its panes stay when their program ends, so that a failed
// test shows what each one printed. The server ends when the test does, and its panes with it. Its
// first pane ends it too if this test binary dies before its cleanup can, as when mise stops the
// tests because another task failed.
func serve(t *testing.T) {
	t.Helper()

	t.Setenv("SKELETON_TEST_PID", strconv.Itoa(os.Getpid()))
	ran(t, exec.CommandContext(t.Context(), "tmux", "-f", "/dev/null", "new-session", "-d",
		"-s", "baton-test-skeleton", "/bin/sh", "-c",
		`while kill -0 "$SKELETON_TEST_PID" 2>/dev/null; do sleep 1; done; tmux kill-server`,
		";", "set-option", "-g", "remain-on-exit", "on"))

	t.Cleanup(func() {
		ctx := context.Background()

		if t.Failed() {
			screens, err := exec.CommandContext(ctx, "tmux",
				"capture-pane", "-p", "-J", "-S", "-", "-t", "=baton-repo-demo-1:0.0", ";",
				"capture-pane", "-p", "-J", "-S", "-", "-t", "=baton-repo-demo-1:0.1", ";",
				"capture-pane", "-p", "-J", "-S", "-", "-t", "=baton-repo-demo-1:0.2", ";",
				"capture-pane", "-p", "-J", "-S", "-", "-t", "=baton-repo-demo-1:0.3",
			).CombinedOutput()
			t.Logf("the panes' screens (%v):\n%s", err, screens)
		}

		_ = exec.CommandContext(ctx, "tmux", "kill-server").Run()
	})
}

// ran runs the command and returns what it printed. It fails the test if the command fails or
// prints anything on its stderr.
func ran(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()

	var stderr strings.Builder

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil || stderr.Len() > 0 {
		t.Fatalf("%s: %v\n%s", cmd, err, stderr.String())
	}

	return string(out)
}

// wantOut runs the command, and fails the test unless it prints what it should.
func wantOut(t *testing.T, cmd *exec.Cmd, want string) {
	t.Helper()

	if got := ran(t, cmd); got != want {
		t.Errorf("%s printed:\n%s\nwant:\n%s", cmd, got, want)
	}
}

// wantPanes fails the test unless the feature's session has the roles' four panes, in order, each
// titled with its role and running baton pane from this test binary: the panes run the baton that
// started the feature, by its path, which on a Mac is the link that started it (decision 42).
func wantPanes(t *testing.T) {
	t.Helper()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		t.Fatal(err)
	}

	var got, want strings.Builder

	for line := range strings.Lines(ran(t, exec.CommandContext(t.Context(), "tmux",
		"list-panes", "-s", "-t", "=baton-repo-demo-1", "-F", "#{pane_title} #{pane_start_command}",
	))) {
		title, command, _ := strings.Cut(line, " ")
		path, args, _ := strings.Cut(command, " ")

		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}

		fmt.Fprintf(&got, "%s %s %s", title, path, args)
	}

	for _, role := range []string{"lead", "engineer", "qa", "reviewer"} {
		fmt.Fprintf(&want, "%s %s pane\n", role, self)
	}

	if got.String() != want.String() {
		t.Errorf("the panes:\n%s\nwant:\n%s", got.String(), want.String())
	}
}

// wantFile fails the test unless the file holds the text.
func wantFile(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != want {
		t.Errorf("%s holds:\n%s\nwant:\n%s", path, got, want)
	}
}

// logged is an event of a feature's journal, as its kind, role, verdict and note's name, and its
// note's text.
type logged struct{ event, note string }

// wantJournal fails the test unless the journal in the feature's folder holds the events, in
// order, with their notes.
func wantJournal(t *testing.T, feature string, want []logged) {
	t.Helper()

	data, err := os.ReadFile(filepath.Clean(filepath.Join(feature, "events.ndjson")))
	if err != nil {
		t.Fatal(err)
	}

	var got []logged

	for line := range strings.Lines(string(data)) {
		var event state.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}

		var note []byte
		if event.Note != "" {
			note, err = os.ReadFile(filepath.Clean(filepath.Join(feature, "notes", event.Note)))
			if err != nil {
				t.Fatal(err)
			}
		}

		got = append(got, logged{
			event: fmt.Sprintf("%s %s %s %s", event.Kind, event.Role, event.Verdict, event.Note),
			note:  string(note),
		})
	}

	if !slices.Equal(got, want) {
		t.Errorf("journal:\n%q\nwant:\n%q", got, want)
	}
}

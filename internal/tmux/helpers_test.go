package tmux_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/tmux"
)

// role is the tests' pane command: it shows the folder that the pane started in and the pane's
// BATON_TMUX_TEST, then reads its terminal, so that it ends with it even if it misses the hangup
// (fact 22).
const role = `printf '%s|%s\n' "$(pwd -P)" "${BATON_TMUX_TEST-unset}"; read -r _`

// sockets numbers the tests' tmux servers.
var sockets atomic.Int64

// TestMain keeps the tests off the user's tmux server: the tests' servers keep their sockets in a
// folder of their own, and no TMUX names the user's server.
func TestMain(m *testing.M) {
	// macOS limits a socket's path to 104 bytes, and $TMPDIR there leaves too few of them.
	dir, err := os.MkdirTemp("/tmp", "baton-tmux-")
	if err == nil {
		err = errors.Join(os.Setenv("TMUX_TMPDIR", dir), os.Unsetenv("TMUX"),
			os.Unsetenv("TMUX_PANE"))
	}

	code := 1
	if err == nil {
		code = m.Run()
		err = os.RemoveAll(dir)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		code = max(code, 1)
	}

	os.Exit(code)
}

// server returns a tmux server of the test's own, which reads only the given config lines, and
// ends it when the test ends.
func server(t *testing.T, config ...string) tmux.Server {
	t.Helper()

	file := filepath.Join(t.TempDir(), "tmux.conf")

	err := os.WriteFile(file, []byte(strings.Join(config, "\n")+"\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	srv := tmux.Server{Socket: fmt.Sprintf("baton-test-%d", sockets.Add(1)), Config: file}

	t.Cleanup(func() {
		// The server is gone if no test step started it or a pane's end ended it.
		_ = command(context.Background(), srv, "kill-server").Run()
	})

	return srv
}

// command returns tmux, to run on the server with the arguments.
func command(ctx context.Context, srv tmux.Server, args ...string) *exec.Cmd {
	global := []string{"-L", srv.Socket, "-f", srv.Config}

	//nolint:gosec // G204: the tests' own tmux and arguments, on their own server (decision 34).
	return exec.CommandContext(ctx, "tmux", append(global, args...)...)
}

// on runs tmux on the server and returns what it printed, without the last newline.
func on(t *testing.T, srv tmux.Server, args ...string) string {
	t.Helper()

	out, err := command(t.Context(), srv, args...).Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		t.Fatalf("tmux %s: %v: %s", strings.Join(args, " "), err, exit.Stderr)
	}

	if err != nil {
		t.Fatalf("tmux %s: %v", strings.Join(args, " "), err)
	}

	return strings.TrimSuffix(string(out), "\n")
}

// shows waits until the pane shows the line, for up to ten seconds. The line may have scrolled
// into the pane's history, as tiling narrowed the pane.
func shows(t *testing.T, srv tmux.Server, pane, line string) {
	t.Helper()

	var screen string

	for range 1000 {
		screen = on(t, srv, "capture-pane", "-p", "-J", "-S", "-", "-t", pane)
		for got := range strings.Lines(screen) {
			if strings.TrimRight(got, " \n") == line {
				return
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("pane %s shows %q; want the line %q", pane, screen, line)
}

// pane returns a pane that runs role, with the title and the environment.
func pane(title string, env ...string) tmux.Pane {
	return tmux.Pane{Title: title, Env: env, Command: []string{"sh", "-c", role}}
}

// panes returns the IDs of the session's panes, by their titles.
func panes(t *testing.T, srv tmux.Server, session string) map[string]string {
	t.Helper()

	ids := map[string]string{}

	listed := on(t, srv, "list-panes", "-s", "-t", "="+session, "-F", "#{pane_id} #{pane_title}")
	for line := range strings.Lines(listed) {
		id, title, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		ids[title] = id
	}

	return ids
}

// folder returns a new folder for the test, as the panes' shell shows it, with symlinks resolved.
func folder(t *testing.T) (dir, shown string) {
	t.Helper()

	dir = t.TempDir()

	shown, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	return dir, shown
}

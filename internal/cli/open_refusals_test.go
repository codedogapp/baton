package cli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStartRefuses opens the orchestrator where baton can't, and checks that baton start stops at
// the first part that fails. It has a home folder of its own, for the config file.
func TestStartRefuses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	program := onPath(t, "tmux")
	mainDir, linked := repos(t)

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	elsewhere := filepath.Join(filepath.Dir(outside), "main")
	inTmux := map[string]string{"TMUX": "/private/tmp/tmux-501/default,1234,0"}
	running := fakeTmux{dirs: map[string]string{"baton-main": mainDir}}
	failing := errors.New("exec format error")
	baton := filepath.Join(mainDir, ".git", "baton")

	for _, test := range []opening{
		{
			name: "in a role's pane", dir: linked, vars: paneOf("lead"),
			want: refused("run it in your own shell, not in a pane that baton started: " +
				"BATON_ROLE names the lead"),
		},
		{
			name: "in the orchestrator's pane", dir: mainDir, vars: orchestratorPane(),
			want: refused("run it in your own shell, not in a pane that baton started: " +
				"BATON_ROLE names the orchestrator"),
		},
		{
			name: "with an argument", args: []string{"start", "now"}, dir: linked,
			want: refused(`unexpected argument "now"`),
		},
		{
			name: "outside a git repo", dir: outside,
			want: refused("find the main worktree: fatal: not a git repository " +
				"(or any of the parent directories): .git"),
		},
		{
			name: "with another repo's session", dir: linked,
			server: fakeTmux{dirs: map[string]string{"baton-main": elsewhere}},
			want: refused("tmux session baton-main runs in " + elsewhere + ", not in this repo, " +
				mainDir + ": baton names a repo's session after its folder"),
		},
		{
			name: "without its own program", dir: linked, self: errors.New("no such file"),
			want: refused("find baton's own program: no such file"),
		},
		{
			name: "with a mistake in the config file", dir: linked,
			setup: func(t *testing.T) { writeConfig(t, `{"roundCap": -1}`) },
			want:  refused("config file " + configPath(t) + ": roundCap -1: want 1 or more"),
		},
		{
			name: "without a gate", dir: linked,
			setup: func(t *testing.T) { writeConfig(t, `{"roundCap": 2}`) },
			want:  refused(strings.TrimSuffix(noGate(t), "\n")),
		},
		{
			name: "with an orchestrator's file that it can't read", dir: linked,
			setup: func(t *testing.T) { damage(t, baton, "orchestrator.json") },
			want: refused("orchestrator file " + filepath.Join(baton, "orchestrator.json") +
				newerFormat),
		},
		{
			name: "with a feature's file that it can't read", dir: linked,
			setup: func(t *testing.T) { damage(t, baton, "features", "demo-2", "feature.json") },
			want: refused("feature file " + filepath.Join(baton, "features", "demo-2",
				"feature.json") + newerFormat),
		},
		{
			name: "when tmux can't make the session", dir: linked,
			server: fakeTmux{making: func(context.Context) error {
				return errors.New("tmux new-session: duplicate session: baton-main")
			}},
			want: refused("tmux new-session: duplicate session: baton-main"),
			made: orchestratorIn("baton-main", mainDir),
		},
		{
			name: "without tmux", dir: linked, server: running,
			setup: func(t *testing.T) { onlyGit(t) },
			want: refused("attach to tmux session baton-main: " +
				`exec: "tmux": executable file not found in $PATH`),
		},
		{
			name: "when tmux can't attach", dir: linked, server: running, exec: failing,
			want: refused("attach to tmux session baton-main: exec format error"),
			line: attachLine(program, "baton-main"),
		},
		{
			name: "when tmux can't switch the client", dir: linked, vars: inTmux,
			server: fakeTmux{dirs: running.dirs, switching: errors.New(
				"tmux switch-client: no current client")},
			want:  refused("tmux switch-client: no current client"),
			shown: []string{"baton-main"},
		},
	} {
		t.Run(test.name, func(t *testing.T) { wantOpening(t, test) })
	}
}

// newerFormat ends the reason that baton gives for a file in a newer format than its own.
const newerFormat = ": format 2 is newer than this baton's, 1: run the newer baton"

// damage writes a file in a newer format than baton's at the path in baton's folder, and returns
// the file's path. It removes baton's folder when the test ends, so the next test finds the repo
// as it was.
func damage(t *testing.T, baton string, path ...string) string {
	t.Helper()

	file := filepath.Join(append([]string{baton}, path...)...)

	t.Cleanup(func() { _ = os.RemoveAll(baton) })

	return write(t, mkdir(t, filepath.Dir(file)), filepath.Base(file), `{"format": 2}`)
}

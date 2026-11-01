package cli_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/tmux"
)

// opening is a run of baton start, or of plain baton, and how it should go.
type opening struct {
	name   string
	args   []string           // baton's arguments, if any.
	dir    string             // The working folder.
	vars   map[string]string  // The environment's variables.
	server fakeTmux           // The user's tmux server, with the sessions that run already.
	setup  func(t *testing.T) // What the test changes first, if anything.
	self   error              // Why baton can't find its own program, if it can't.
	exec   error              // Why tmux can't attach the terminal, if it can't.
	want   result
	made   []tmux.Session // The sessions that baton asked tmux to make.
	shown  []string       // The sessions that baton switched the user's client to.
	line   *execed        // The command line that baton gave Env.Exec, if any.
}

// TestStartOpensTheOrchestrator opens the orchestrator from the repo's worktrees, outside tmux and
// inside it, with the orchestrator's session running and without it. baton start makes the
// session in the main worktree if it isn't running, then shows it: outside tmux, tmux takes
// baton's place and attaches the terminal to it, and inside, the user's client switches to it.
func TestStartOpensTheOrchestrator(t *testing.T) {
	program := onPath(t, "tmux")
	mainDir, linked := repos(t)
	dotted := repoNamed(t, "my.repo")

	inTmux := map[string]string{"TMUX": "/private/tmp/tmux-501/default,1234,0"}
	running := fakeTmux{dirs: map[string]string{"baton-main": mainDir}}
	started := result{stdout: "Started the orchestrator in tmux session baton-main.\n"}
	attached := attachLine(program, "baton-main")

	for _, test := range []opening{
		{
			name: "plain baton", dir: mainDir,
			want: started, made: orchestratorIn("baton-main", mainDir), line: attached,
		},
		{
			name: "from a linked worktree", args: []string{"start"}, dir: linked,
			want: started, made: orchestratorIn("baton-main", mainDir), line: attached,
		},
		{
			name: "in a repo whose name holds a dot", args: []string{"start"}, dir: dotted,
			want: result{stdout: "Started the orchestrator in tmux session baton-my_repo.\n"},
			made: orchestratorIn("baton-my_repo", dotted), line: attachLine(program, "baton-my_repo"),
		},
		{
			name: "with its session running", args: []string{"start"}, dir: linked,
			server: running, line: attached,
		},
		{
			name: "inside tmux", args: []string{"start"}, dir: linked, vars: inTmux,
			want: started, made: orchestratorIn("baton-main", mainDir),
			shown: []string{"baton-main"},
		},
		{
			name: "inside tmux, with its session running", dir: mainDir, vars: inTmux,
			server: running, shown: []string{"baton-main"},
		},
	} {
		t.Run(test.name, func(t *testing.T) { wantOpening(t, test) })
	}
}

// TestStartRefuses opens the orchestrator where baton can't, and checks that baton start stops at
// the first part that fails.
func TestStartRefuses(t *testing.T) {
	program := onPath(t, "tmux")
	mainDir, linked := repos(t)

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	elsewhere := filepath.Join(filepath.Dir(outside), "main")
	inTmux := map[string]string{"TMUX": "/private/tmp/tmux-501/default,1234,0"}
	running := fakeTmux{dirs: map[string]string{"baton-main": mainDir}}
	failing := errors.New("exec format error")

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

// onlyGit leaves the user's git alone on the PATH, until the test ends.
func onlyGit(t *testing.T) {
	t.Helper()

	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin)
}

// attachLine returns the command line that has tmux, the stand-in at the path, attach the
// terminal to the session, on the fake server.
func attachLine(program, name string) *execed {
	return &execed{
		path: program, args: []string{"tmux", "-L", "fake", "attach-session", "-t", "=" + name},
	}
}

// orchestratorIn returns the orchestrator's session, with the name, in the main worktree.
func orchestratorIn(name, mainDir string) []tmux.Session {
	return []tmux.Session{{Name: name, Dir: mainDir, Panes: []tmux.Pane{{
		Title:   "orchestrator",
		Env:     []string{"BATON_ROLE=orchestrator"},
		Command: []string{self, "pane"},
	}}}}
}

// refused is how baton start ends when it refuses for the reason.
func refused(reason string) result {
	return result{code: 1, stderr: "baton start: " + reason + "\n"}
}

// wantOpening runs baton as the test says, and checks how it went.
func wantOpening(t *testing.T, test opening) {
	t.Helper()

	if test.setup != nil {
		test.setup(t)
	}

	server := test.server
	env := cli.Env{
		Args:       test.args,
		Getenv:     func(name string) string { return test.vars[name] },
		Dir:        test.dir,
		Executable: func() (string, error) { return self, test.self },
		Tmux:       &server,
	}

	got := runExec(t.Context(), env, test.exec)
	if got.result != test.want {
		t.Errorf("got %+v; want %+v", got.result, test.want)
	}

	if !reflect.DeepEqual(server.sessions, test.made) {
		t.Errorf("tmux made %+v; want %+v", server.sessions, test.made)
	}

	if !slices.Equal(server.shown, test.shown) {
		t.Errorf("the user's client switched to %q; want %q", server.shown, test.shown)
	}

	if test.line == nil && len(got.execs) > 0 {
		t.Errorf("Exec got %q; want no command line", got.execs)
	} else if test.line != nil {
		wantExec(
			t,
			got.execs,
			execed{path: test.line.path, args: test.line.args, env: os.Environ()},
		)
	}
}

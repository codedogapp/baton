package cli_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
)

// The tests of the commands that hand over to another program, baton pane and baton start, run
// stand-ins for the programs, and record the command lines that baton gives Env.Exec.

// execed is a command line that baton gave Env.Exec, to replace baton with.
type execed struct {
	path string
	args []string
	env  []string
}

// ran is how a run of baton went, with the command lines that it gave Env.Exec.
type ran struct {
	result

	execs []execed
}

// onPath puts a stand-in for the program first on the PATH, which does nothing if it runs, and
// returns its path.
func onPath(t *testing.T, name string) string {
	t.Helper()

	program, err := exec.LookPath("true")
	if err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	path := filepath.Join(bin, name)

	if err := os.Symlink(program, path); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin+string(filepath.ListSeparator)+os.Getenv("PATH"))

	return path
}

// runExec runs baton in the environment until the context ends, with its output captured, and an
// Exec that records each command line and returns the error. A nil error stands for a program
// that has replaced baton.
func runExec(ctx context.Context, env cli.Env, fails error) ran {
	var (
		stdout, stderr strings.Builder
		got            ran
	)

	env.Stdout, env.Stderr = &stdout, &stderr
	env.Exec = func(path string, args, vars []string) error {
		got.execs = append(got.execs, execed{path: path, args: args, env: vars})

		return fails
	}

	got.code = cli.Run(ctx, env)
	got.stdout, got.stderr = stdout.String(), stderr.String()

	return got
}

// wantExec checks that baton gave Env.Exec the command line, with the environment, and no other.
func wantExec(t *testing.T, execs []execed, want execed) {
	t.Helper()

	if len(execs) != 1 {
		t.Fatalf("Exec got %d command lines; want 1", len(execs))
	}

	line := execs[0]
	if line.path != want.path || !slices.Equal(line.args, want.args) {
		t.Errorf("Exec got %q %q; want %q %q", line.path, line.args, want.path, want.args)
	}

	if !slices.Equal(line.env, want.env) {
		t.Errorf("Exec got another environment: with %q, and without %q",
			without(line.env, want.env), without(want.env, line.env))
	}
}

// without returns the variables, without those in the other list.
func without(vars, other []string) []string {
	return slices.DeleteFunc(slices.Clone(vars), func(variable string) bool {
		return slices.Contains(other, variable)
	})
}

package cli_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The feature that the tests hand over.
const slug = "demo-1"

// TestMain runs git without the user's config, in the C locale, with a test author: baton's own
// git too, which makes the features' branches and worktrees. It clears the variables that git sets
// for its hooks, such as GIT_DIR, which would point git at the repo that runs the hook.
func TestMain(m *testing.M) {
	var errs []error

	for _, env := range os.Environ() {
		if name, _, _ := strings.Cut(env, "="); strings.HasPrefix(name, "GIT_") {
			errs = append(errs, os.Unsetenv(name))
		}
	}

	for _, env := range []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C",
		"GIT_AUTHOR_NAME=baton", "GIT_AUTHOR_EMAIL=baton@example.com",
		"GIT_COMMITTER_NAME=baton", "GIT_COMMITTER_EMAIL=baton@example.com",
	} {
		name, value, _ := strings.Cut(env, "=")
		errs = append(errs, os.Setenv(name, value))
	}

	if err := errors.Join(errs...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// repos makes a git repo with a commit, and a linked worktree next to it, like a feature's. It
// returns the folders of the main worktree and of the linked one, with symlinks resolved, as git
// gives them (fact 21).
func repos(t *testing.T) (mainDir, linked string) {
	t.Helper()

	mainDir = repoNamed(t, "main")
	gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "worktree", "add", "--quiet",
		"../linked"))

	return mainDir, filepath.Join(filepath.Dir(mainDir), "linked")
}

// repoNamed makes a git repo with a commit, in a new folder with the name, and returns the folder,
// with symlinks resolved.
func repoNamed(t *testing.T, name string) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	dir = filepath.Join(dir, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	gitIn(t, dir, exec.CommandContext(t.Context(), "git", "init", "--quiet"))
	gitIn(t, dir, exec.CommandContext(t.Context(), "git", "commit", "--quiet", "--allow-empty",
		"--message", "Start"))

	return dir
}

// gitIn runs the git command in the folder, and returns what it printed. Its callers spell out
// git's arguments, so the tests run no command that they don't show.
func gitIn(t *testing.T, dir string, cmd *exec.Cmd) string {
	t.Helper()

	var stderr strings.Builder

	cmd.Dir, cmd.Stderr = dir, &stderr

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v\n%s", cmd, err, stderr.String())
	}

	return string(out)
}

// fresh is a new feature's flow: the lead holds the orchestrator's brief.
func fresh() flow.State {
	return flow.State{Holder: flow.RoleLead, Status: flow.StatusRunning, Cap: 3}
}

// started stores the feature demo-1, where it stands in the flow, in baton's folder of the repo,
// and returns the store.
func started(t *testing.T, mainDir string, now flow.State) state.Store {
	t.Helper()

	store := state.Store{Dir: filepath.Join(mainDir, ".git", "baton")}
	add(t, store, state.Feature{Slug: slug, Flow: now})

	return store
}

// add stores the new feature, with the orchestrator's brief.
func add(t *testing.T, store state.Store, feature state.Feature) {
	t.Helper()

	entry := state.Entry{
		Event: state.Event{Kind: state.EventStart, Role: flow.RoleOrchestrator},
		Note:  []byte("Build the demo.\n"),
	}

	if err := store.Create(feature, entry); err != nil {
		t.Fatal(err)
	}
}

// paneOf returns the environment of the role's pane in the feature.
func paneOf(role flow.Role) map[string]string {
	return map[string]string{"BATON_ROLE": string(role), "BATON_FEATURE": slug}
}

// result is how a run of baton went.
type result struct {
	code   int
	stdout string
	stderr string
}

// run runs baton with the arguments, in the folder and with the environment.
func run(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()

	return runUntil(t.Context(), dir, env, args...)
}

// runUntil runs baton like run, until the context ends.
func runUntil(ctx context.Context, dir string, env map[string]string, args ...string) result {
	return runEnv(ctx, cli.Env{
		Args:   args,
		Getenv: func(name string) string { return env[name] },
		Dir:    dir,
	})
}

// runEnv runs baton in the environment, until the context ends, and returns what it printed.
func runEnv(ctx context.Context, env cli.Env) result {
	var stdout, stderr strings.Builder

	env.Stdout, env.Stderr = &stdout, &stderr
	code := cli.Run(ctx, env)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// write writes the text to the file in the folder, and returns the file's path.
func write(t *testing.T, dir, name, text string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// snapshot returns the files in the folder and in its subfolders, each with its content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	files := map[string]string{}

	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		data, err := fs.ReadFile(root.FS(), path)
		files[path] = string(data)

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	return files
}

package cli_test

import (
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

// repos makes a git repo with a commit, and a linked worktree next to it, like a feature's. It
// returns the folders of the main worktree and of the linked one. git runs without the user's
// config.
func repos(t *testing.T) (mainDir, linked string) {
	t.Helper()

	dir := t.TempDir()
	mainDir, linked = filepath.Join(dir, "main"), filepath.Join(dir, "linked")

	if err := os.Mkdir(mainDir, 0o700); err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	for _, cmd := range []*exec.Cmd{
		exec.CommandContext(ctx, "git", "init", "--quiet"),
		exec.CommandContext(ctx, "git", "-c", "user.name=baton", "-c", "user.email=baton@example.com",
			"commit", "--quiet", "--allow-empty", "--message", "Start"),
		exec.CommandContext(ctx, "git", "worktree", "add", "--quiet", "../linked"),
	} {
		cmd.Dir = mainDir

		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", cmd, err, out)
		}
	}

	return mainDir, linked
}

// fresh is a new feature's flow: the lead holds the orchestrator's brief.
func fresh() flow.State {
	return flow.State{Holder: flow.RoleLead, Status: flow.StatusRunning, Cap: 3}
}

// started stores the feature, where it stands in the flow, in baton's folder of the repo, and
// returns the store.
func started(t *testing.T, mainDir string, now flow.State) state.Store {
	t.Helper()

	store := state.Store{Dir: filepath.Join(mainDir, ".git", "baton")}
	entry := state.Entry{
		Event: state.Event{Kind: state.EventStart, Role: flow.RoleOrchestrator},
		Note:  []byte("Build the demo.\n"),
	}

	if err := store.Create(state.Feature{Slug: slug, Flow: now}, entry); err != nil {
		t.Fatal(err)
	}

	return store
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

	var stdout, stderr strings.Builder

	code := cli.Run(t.Context(), cli.Env{
		Args:   args,
		Getenv: func(name string) string { return env[name] },
		Dir:    dir,
		Stdout: &stdout,
		Stderr: &stderr,
	})

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

package cli_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
	"github.com/codedogapp/baton/internal/tmux"
)

// self is the path of baton's own program, in the tests.
const self = "/opt/baton/bin/baton"

// fakeTmux stands in for the user's tmux server: it keeps the sessions that baton asks for.
type fakeTmux struct {
	sessions []tmux.Session
	making   func(ctx context.Context) error // What making a session does and returns, if set.
}

func (fake *fakeTmux) NewSession(ctx context.Context, session tmux.Session) error {
	fake.sessions = append(fake.sessions, session)

	if fake.making == nil {
		return nil
	}

	return fake.making(ctx)
}

// startIn returns the environment of baton feature start with the arguments, in the folder,
// outside any pane, with self as baton's own program and the fake as the user's tmux server.
func startIn(dir string, fake *fakeTmux, args ...string) cli.Env {
	return cli.Env{
		Args:       append([]string{"feature", "start"}, args...),
		Getenv:     func(string) string { return "" },
		Dir:        dir,
		Executable: func() (string, error) { return self, nil },
		Tmux:       fake,
	}
}

// rolePane returns the pane of the role in feature demo-1's session, which runs baton pane.
func rolePane(role string) tmux.Pane {
	return tmux.Pane{
		Title:   role,
		Env:     []string{"BATON_ROLE=" + role, "BATON_FEATURE=demo-1"},
		Command: []string{self, "pane"},
	}
}

// repoState returns the repo's worktrees, and its branches with their commits, as git lists them.
func repoState(t *testing.T, dir string) string {
	t.Helper()

	return gitIn(t, dir, exec.CommandContext(t.Context(), "git", "worktree", "list",
		"--porcelain")) +
		gitIn(t, dir, exec.CommandContext(t.Context(), "git", "for-each-ref",
			"--format=%(refname) %(objectname)", "refs/heads"))
}

// wantSameRepo checks that git lists the worktrees and branches that it listed before.
func wantSameRepo(t *testing.T, mainDir, before string) {
	t.Helper()

	if after := repoState(t, mainDir); after != before {
		t.Errorf("git lists\n%s\nwant what it listed before:\n%s", after, before)
	}
}

// TestStartFeature starts a feature in the orchestrator's pane, from a linked worktree whose
// HEAD isn't the main worktree's.
func TestStartFeature(t *testing.T) {
	mainDir, linked := repos(t)
	gitIn(t, linked, exec.CommandContext(t.Context(), "git", "commit", "--quiet", "--allow-empty",
		"--message", "Not the base"))
	write(t, linked, "brief.md", "Build the demo.\n")

	fake := &fakeTmux{}
	env := startIn(linked, fake, slug, "--brief", "brief.md")
	env.Getenv = func(name string) string {
		return map[string]string{"BATON_ROLE": "orchestrator"}[name]
	}

	worktree := mainDir + "-demo-1"
	want := result{stdout: "Started demo-1: branch feature/demo-1, worktree " + worktree +
		", tmux session baton-main-demo-1. The lead has the brief.\n"}

	if got := runEnv(t.Context(), env); got != want {
		t.Fatalf("baton feature start = %+v, want %+v", got, want)
	}

	session := tmux.Session{Name: "baton-main-demo-1", Dir: worktree, Panes: []tmux.Pane{
		rolePane("lead"), rolePane("engineer"), rolePane("qa"), rolePane("reviewer"),
	}}
	if !reflect.DeepEqual(fake.sessions, []tmux.Session{session}) {
		t.Errorf("tmux sessions:\n%+v\nwant:\n%+v", fake.sessions, session)
	}

	store := state.Store{Dir: filepath.Join(mainDir, ".git", "baton")}

	feature, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	if want := (state.Feature{
		Slug: slug, Branch: "feature/demo-1", Worktree: worktree, Flow: fresh(),
		Work: state.Work{Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator},
	}); feature != want {
		t.Errorf("feature = %+v, want %+v", feature, want)
	}

	wantJournal(t, store, []logged{
		{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
	})

	base := gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD"))
	listed := "worktree " + worktree + "\nHEAD " + base + "branch refs/heads/feature/demo-1\n"

	if got := repoState(t, mainDir); !strings.Contains(got, listed) {
		t.Errorf("git lists\n%s\nwant it to list\n%s", got, listed)
	}
}

// TestStartFeatureNamesItsPartsAfterTheRepo starts a feature outside any pane, in a repo whose
// name holds a dot, which a tmux session's name can't hold.
func TestStartFeatureNamesItsPartsAfterTheRepo(t *testing.T) {
	mainDir := repoNamed(t, "my.repo")
	brief := write(t, t.TempDir(), "brief.md", "Build the demo.\n")

	fake := &fakeTmux{}
	got := runEnv(t.Context(), startIn(mainDir, fake, "--brief", brief, slug))

	worktree := filepath.Join(filepath.Dir(mainDir), "my.repo-demo-1")
	want := result{stdout: "Started demo-1: branch feature/demo-1, worktree " + worktree +
		", tmux session baton-my_repo-demo-1. The lead has the brief.\n"}

	if got != want {
		t.Errorf("baton feature start = %+v, want %+v", got, want)
	}

	if len(fake.sessions) != 1 || fake.sessions[0].Name != "baton-my_repo-demo-1" ||
		fake.sessions[0].Dir != worktree {
		t.Errorf("tmux sessions: %+v; want baton-my_repo-demo-1 in %s", fake.sessions, worktree)
	}
}

package cli_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestFeatureCloseEndsTheTeamMidWork closes demo-1 while the lead's question waits for its answer
// and the engineer waits for work. The close ends the feature's tmux session, and with it the
// lead's ask and the engineer's wait, and gives the feature to the orchestrator, closed, at its
// round, with no work and no question. It keeps the feature's branch and its worktree. Then
// nothing hands the feature over, no work can come to the orchestrator, and each of the feature's
// copilots may stop.
func TestFeatureCloseEndsTheTeamMidWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		demo := newTeam(t)

		asked, waited := make(chan result, 1), make(chan result, 1)
		go func() { asked <- leadAsks(t, demo.linked, question) }()
		go func() { waited <- waitAs(t, demo.linked, flow.RoleEngineer) }()

		synctest.Wait()

		got, want := demo.closeFrom(t, orchestratorPane(), slug), result{
			stdout: "Closed demo-1: ended tmux session baton-linked; kept branch linked and " +
				"worktree " + demo.linked + ".\n",
		}
		if got != want {
			t.Errorf("got %+v; want %+v", got, want)
		}

		want = result{code: 1, stderr: "baton ask: demo-1 is closed, so no answer comes\n"}
		if got := <-asked; got != want {
			t.Errorf("the lead's ask: got %+v; want %+v", got, want)
		}

		want = result{
			code: 1, stderr: "baton wait: demo-1 is closed, so no work comes to the engineer\n",
		}
		if got := <-waited; got != want {
			t.Errorf("the engineer's wait: got %+v; want %+v", got, want)
		}

		demo.wantClosed(t, []string{"baton-linked"})
		wantJournal(t, demo.store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"question lead  ", question},
			{"close orchestrator  ", ""},
		})
		wantNothingMoves(t, demo)
	})
}

// wantNothingMoves checks that nothing hands over the closed demo-1, that no work can come to the
// orchestrator, and that each of the feature's copilots may stop.
func wantNothingMoves(t *testing.T, demo team) {
	t.Helper()

	note := write(t, t.TempDir(), "note.md", "Go on.\n")
	closed := "baton handover: the feature is closed, and nothing hands it over again\n"

	cases := []struct {
		name string
		got  result
		want string // What baton printed to stderr.
	}{
		{
			"the lead's handover",
			run(t, demo.linked, paneOf(flow.RoleLead), "handover", "--note", note), closed,
		},
		{
			"the orchestrator's handover",
			run(t, demo.mainDir, orchestratorPane(), "handover", slug, "--to", "lead", "--note",
				note),
			closed,
		},
		{
			"the orchestrator's wait", runFor(t, demo.mainDir, orchestratorPane(), "wait"),
			"baton wait: no feature is running, so no work can come to the orchestrator\n",
		},
	}

	for _, test := range cases {
		if test.got != (result{code: 1, stderr: test.want}) {
			t.Errorf("%s: got %+v; want exit code 1 and %q", test.name, test.got, test.want)
		}
	}

	for _, pane := range []map[string]string{
		paneOf(flow.RoleLead), paneOf(flow.RoleEngineer), paneOf(flow.RoleQA),
		paneOf(flow.RoleReviewer), orchestratorPane(),
	} {
		wantStop(t, stopHook(t, demo.linked, pane, hookInput(t, demo.linked, false)), "")
	}
}

// TestFeatureCloseFinishesWhatFailed has the end of the feature's tmux session fail, then the
// removal of its worktree, which holds a file that no commit has. Each close says what stays, and
// the next one does it, while the feature stays closed, once.
func TestFeatureCloseFinishesWhatFailed(t *testing.T) {
	demo := newTeam(t)
	demo.sessions.killing = errors.New("tmux kill-session: server exited unexpectedly")

	got, want := demo.closeFrom(t, nil, slug, "--remove-worktree"), result{
		code: 1, stderr: "baton feature close: demo-1 is closed, but its tmux session runs: " +
			"tmux kill-session: server exited unexpectedly\n",
	}
	if got != want {
		t.Errorf("the first close: got %+v; want %+v", got, want)
	}

	demo.sessions.killing = nil
	draft := write(t, demo.linked, "draft.md", "A draft.\n")

	got, want = demo.closeFrom(t, nil, slug, "--remove-worktree"), result{
		code: 1, stderr: "baton feature close: demo-1 is closed, and ended tmux session " +
			"baton-linked, but its worktree stays: remove the worktree " + demo.linked +
			": fatal: '" + demo.linked + "' contains modified or untracked files, " +
			"use --force to delete it\n",
	}
	if got != want {
		t.Errorf("the second close: got %+v; want %+v", got, want)
	}

	if err := os.Remove(draft); err != nil {
		t.Fatal(err)
	}

	got, want = demo.closeFrom(t, nil, "--remove-worktree", slug), result{
		stdout: "Closed demo-1: tmux session baton-linked had ended; removed worktree " +
			demo.linked + " and kept branch linked.\n",
	}
	if got != want {
		t.Errorf("the third close: got %+v; want %+v", got, want)
	}

	if _, err := os.Stat(demo.linked); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the worktree: got %v; want it gone", err)
	}

	gitIn(t, demo.mainDir, exec.CommandContext(t.Context(), "git", "rev-parse", "--verify",
		"--quiet", "refs/heads/linked"))
	demo.wantClosed(t, []string{"baton-linked", "baton-linked"})
	wantJournal(t, demo.store, []logged{
		{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
		{"close orchestrator  ", ""},
	})
}

// TestFeatureCloseForgetsAWorktreeWhoseFolderIsGone removes the worktree of demo-1 after the user
// removed its folder: git only forgets its record of the worktree (fact 46).
func TestFeatureCloseForgetsAWorktreeWhoseFolderIsGone(t *testing.T) {
	demo := newTeam(t)
	if err := os.RemoveAll(demo.linked); err != nil {
		t.Fatal(err)
	}

	got, want := demo.closeFrom(t, orchestratorPane(), slug, "--remove-worktree"), result{
		stdout: "Closed demo-1: ended tmux session baton-linked; removed worktree " +
			demo.linked + " and kept branch linked.\n",
	}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}

	list := gitIn(t, demo.mainDir, exec.CommandContext(t.Context(), "git", "worktree", "list",
		"--porcelain"))
	if strings.Contains(list, "worktree "+demo.linked+"\n") {
		t.Errorf("git worktree list --porcelain:\n%swant no worktree %s", list, demo.linked)
	}

	demo.wantClosed(t, []string{"baton-linked"})
}

// team is the running feature demo-1, on branch linked, in its own worktree, with its tmux
// session, baton-linked, on a fake tmux server.
type team struct {
	mainDir  string
	linked   string // The feature's worktree.
	store    state.Store
	sessions *fakeTmux
}

// newTeam starts demo-1 in a new repo, with the lead holding the orchestrator's brief.
func newTeam(t *testing.T) team {
	t.Helper()

	mainDir, linked := repos(t)
	store := state.Store{Dir: filepath.Join(mainDir, ".git", "baton")}
	add(t, store, state.Feature{Slug: slug, Branch: "linked", Worktree: linked, Flow: fresh()})

	return team{
		mainDir: mainDir, linked: linked, store: store,
		sessions: &fakeTmux{dirs: map[string]string{"baton-linked": linked}},
	}
}

// closeFrom runs baton feature close with the arguments in the pane, from the main worktree.
func (demo team) closeFrom(t *testing.T, pane map[string]string, args ...string) result {
	t.Helper()

	return runEnv(t.Context(), cli.Env{
		Args:   append([]string{"feature", "close"}, args...),
		Getenv: func(name string) string { return pane[name] },
		Dir:    demo.mainDir,
		Tmux:   demo.sessions,
	})
}

// wantClosed checks that demo-1 is closed, at round 0, with no work and no question, and that
// baton ended the tmux sessions, or tried to, in that order.
func (demo team) wantClosed(t *testing.T, killed []string) {
	t.Helper()

	want := state.Feature{
		Slug: slug, Branch: "linked", Worktree: demo.linked,
		Flow: flow.State{Holder: flow.RoleOrchestrator, Status: flow.StatusClosed, Cap: 3},
	}
	if got, err := demo.store.Read(slug); err != nil || got != want {
		t.Errorf("Read = %+v, %v; want %+v", got, err, want)
	}

	if !slices.Equal(demo.sessions.killed, killed) {
		t.Errorf("killed %q; want %q", demo.sessions.killed, killed)
	}
}

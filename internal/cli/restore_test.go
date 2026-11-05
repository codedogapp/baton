package cli_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
	"github.com/codedogapp/baton/internal/tmux"
)

// TestStartStartsTheTeamsAgain opens the orchestrator in a repo with a feature in each status,
// outside tmux and inside it. baton start makes again the session of an open feature that has
// none, starts again the dead panes of one whose session runs, and leaves a closed feature alone,
// then shows the orchestrator's session.
func TestStartStartsTheTeamsAgain(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	program := onPath(t, "tmux")
	mainDir, linked := repos(t)
	team := teams(t, mainDir, flow.StatusRunning, flow.StatusReady, flow.StatusClosed,
		flow.StatusDelivered, flow.StatusEscalated)

	running := map[string]string{team[1].Name: team[1].Dir, team[3].Name: team[3].Dir}
	dead := map[string][]string{team[1].Name: {"engineer", "qa"}}
	restored := again(team[0]) + "Started again in tmux session baton-main-demo-2: engineer, " +
		"qa.\n" + again(team[4])
	orchestrator := "Started the orchestrator in tmux session baton-main.\n"

	for _, test := range []opening{
		{
			name: "after a restart", dir: linked,
			want: result{stdout: orchestrator + again(team[0], team[1], team[3], team[4])},
			made: append(orchestratorIn("baton-main", mainDir), team[0], team[1], team[3], team[4]),
			line: attachLine(program, "baton-main"),
		},
		{
			name: "with the sessions of some running", dir: linked,
			server: fakeTmux{dirs: running, dead: dead},
			want:   result{stdout: orchestrator + restored},
			made:   append(orchestratorIn("baton-main", mainDir), team[0], team[4]),
			line:   attachLine(program, "baton-main"), respawned: []tmux.Session{team[1], team[3]},
		},
		{
			name: "inside tmux, with the orchestrator's session running", dir: mainDir,
			vars:   map[string]string{"TMUX": "/private/tmp/tmux-501/default,1234,0"},
			server: fakeTmux{dirs: with(running, "baton-main", mainDir), dead: dead},
			want:   result{stdout: restored}, made: []tmux.Session{team[0], team[4]},
			shown: []string{"baton-main"}, respawned: []tmux.Session{team[1], team[3]},
		},
	} {
		t.Run(test.name, func(t *testing.T) { wantOpening(t, test) })
	}
}

// TestStartRefusesToHideATeamThatItCantStart opens the orchestrator, whose session runs, where
// baton can't start a feature's team again. It tries every feature, then fails before it shows the
// orchestrator's session, which would hide why.
func TestStartRefusesToHideATeamThatItCantStart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mainDir, linked := repos(t)
	team := teams(t, mainDir, flow.StatusRunning, flow.StatusReady, flow.StatusEscalated)
	baton := filepath.Join(mainDir, ".git", "baton")
	elsewhere := filepath.Join(t.TempDir(), "main-demo-2")

	running := map[string]string{"baton-main": mainDir, team[1].Name: team[1].Dir}
	dead := map[string][]string{team[1].Name: {"engineer", "qa"}}
	failing := func(context.Context) error {
		return errors.New("tmux new-session: duplicate session")
	}

	for _, test := range []opening{
		{
			name: "when tmux can't make a feature's session", dir: linked,
			server: fakeTmux{dirs: running, dead: dead, making: failing},
			want: result{
				code:   1,
				stdout: "Started again in tmux session baton-main-demo-2: engineer, qa.\n",
				stderr: "baton start: start demo-1's team again: tmux new-session: duplicate " +
					"session\nstart demo-3's team again: tmux new-session: duplicate session\n",
			},
			made: []tmux.Session{team[0], team[2]}, respawned: []tmux.Session{team[1]},
		},
		{
			name: "when tmux can't start a dead pane again", dir: linked,
			server: fakeTmux{dirs: running, dead: dead, respawning: errors.New(
				"start the qa pane of tmux session baton-main-demo-2 again: can't find pane: %3")},
			want: result{
				code: 1,
				stdout: again(team[0]) + "Started again in tmux session baton-main-demo-2: " +
					"engineer.\n" + again(team[2]),
				stderr: "baton start: start demo-2's team again: start the qa pane of tmux " +
					"session baton-main-demo-2 again: can't find pane: %3\n",
			},
			made: []tmux.Session{team[0], team[2]}, respawned: []tmux.Session{team[1]},
		},
		{
			name: "with a feature's session in another folder", dir: linked,
			server: fakeTmux{dirs: with(running, team[1].Name, elsewhere)},
			want: result{
				code:   1,
				stdout: again(team[0], team[2]),
				stderr: "baton start: start demo-2's team again: tmux session baton-main-demo-2 " +
					"runs in " + elsewhere + ", not in the worktree of demo-2, " + team[1].Dir +
					": baton changes only the sessions that it made\n",
			},
			made: []tmux.Session{team[0], team[2]},
		},
		{
			name: "without its own program", dir: linked, server: fakeTmux{dirs: running},
			self: errors.New("no such file"), want: refused("find baton's own program: no such file"),
		},
		{
			// The last case, since damage removes baton's folder, with the features, at its end.
			name: "with a feature's file that it can't read", dir: linked,
			server: fakeTmux{dirs: running},
			setup:  func(t *testing.T) { damage(t, baton, "features", "demo-9", "feature.json") },
			want: refused("feature file " + filepath.Join(baton, "features", "demo-9",
				"feature.json") + newerFormat),
		},
	} {
		t.Run(test.name, func(t *testing.T) { wantOpening(t, test) })
	}
}

// teams stores a feature in each status, demo-1 onwards, each with its worktree next to the main
// worktree, and returns the tmux sessions of their teams, as baton makes them.
func teams(t *testing.T, mainDir string, statuses ...flow.Status) []tmux.Session {
	t.Helper()

	store := state.Store{Dir: filepath.Join(mainDir, ".git", "baton")}

	var sessions []tmux.Session

	for i, status := range statuses {
		name := fmt.Sprintf("demo-%d", i+1)
		now := flow.State{Holder: flow.RoleOrchestrator, Status: status, Round: 3, Cap: 3}

		if status == flow.StatusRunning {
			now = fresh()
		}

		worktree := mainDir + "-" + name
		add(t, store, state.Feature{Slug: name, Worktree: worktree, Flow: now})

		session := tmux.Session{Name: "baton-main-" + name, Dir: worktree, Remain: true}
		for _, role := range []string{"lead", "engineer", "qa", "reviewer"} {
			session.Panes = append(session.Panes, tmux.Pane{
				Title:   role,
				Env:     []string{"BATON_ROLE=" + role, "BATON_FEATURE=" + name},
				Command: []string{self, "pane"},
			})
		}

		sessions = append(sessions, session)
	}

	return sessions
}

// again returns what baton start says when it makes the sessions again.
func again(sessions ...tmux.Session) string {
	var said strings.Builder
	for _, session := range sessions {
		said.WriteString("Started again in tmux session " + session.Name + ": lead, engineer, " +
			"qa, reviewer.\n")
	}

	return said.String()
}

// with returns a copy of the sessions' folders, with the session in the folder.
func with(dirs map[string]string, name, dir string) map[string]string {
	dirs = maps.Clone(dirs)
	dirs[name] = dir

	return dirs
}

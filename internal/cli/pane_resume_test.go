package cli_test

import (
	"testing"
	"testing/synctest"
	"time"
	"uuid"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// carryingOn starts the first prompt of a role's copilot whose pane started again while the role
// held the work that it picked up (decision 67).
const carryingOn = "Your pane started again: carry on with this work.\n"

// TestPaneCarriesOnWithTheRolesWork starts the lead's pane again once its copilot has ended, while
// the lead holds the work that it picked up: the pane doesn't wait, says nothing, and becomes the
// lead's copilot at once, in the session that the lead's copilot had, which copilot resumes, with
// the same work after a line that says to carry on with it. It logs that it did, and leaves the
// work picked up (decision 67). The engineer's pane, started then, waits for its own work.
func TestPaneCarriesOnWithTheRolesWork(t *testing.T) {
	copilot, cache := standIn(t)

	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		env, _ := inPane(t, linked, paneOf(flow.RoleLead))
		if got := paneFor(t, env); got.code != 0 {
			t.Fatalf("the lead's first pane: %+v", got)
		}

		session := sessionOf(t, store, flow.RoleLead)
		env, _ = inPane(t, linked, paneOf(flow.RoleLead))

		wantCarryOn(t, paneFor(t, env), execed{
			path: copilot,
			args: copilotLine(flow.RoleLead, pluginIn(t, cache), session,
				carryingOn+leadsBrief(store)),
			env: copilotEnv(),
		})

		env, user := inPane(t, linked, paneOf(flow.RoleEngineer))
		engineer := make(chan ran, 1)

		go func() { engineer <- paneFor(t, env) }()

		time.Sleep(time.Minute)
		stillWaiting(t, engineer)

		if err := user.Close(); err != nil {
			t.Fatal(err)
		}

		wantRefused(t, <-engineer, "engineer in demo-1: waiting for work.\n",
			"stopped waiting: the terminal's input ended: EOF\n")
		wantWork(t, store, state.Work{
			Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator, Picked: true,
		})
		wantJournal(t, store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"pickup lead  ", ""},
			{"resume lead  ", ""},
		})
	})
}

// TestPaneCarriesOnInANewSession starts the lead's pane again while the lead holds the work that
// it picked up, in a feature's file that keeps no session for the lead, as one from before the
// cost step: the pane carries on with the work in a new session, which it keeps as the lead's.
func TestPaneCarriesOnInANewSession(t *testing.T) {
	copilot, cache := standIn(t)

	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())
		edit(t, store, func(feature *state.Feature) { feature.Work.Picked = true })

		env, _ := inPane(t, linked, paneOf(flow.RoleLead))
		got := paneFor(t, env)

		wantCarryOn(t, got, execed{
			path: copilot,
			args: copilotLine(flow.RoleLead, pluginIn(t, cache), sessionOf(t, store, flow.RoleLead),
				carryingOn+leadsBrief(store)),
			env: copilotEnv(),
		})
	})
}

// TestPaneKeepsTheRolesSession starts the lead's pane again while the lead's work waits for it,
// and the feature keeps a session for the lead, as when its copilot ended while another role held
// the feature: the pane waits as before, then picks up the work in that session, so a role has
// one session in a feature (decision 67).
func TestPaneKeepsTheRolesSession(t *testing.T) {
	copilot, cache := standIn(t)

	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		session := uuid.New()
		keep := func(feature *state.Feature) { *feature.Sessions.Of(flow.RoleLead) = session }
		edit(t, store, keep)

		env, _ := inPane(t, linked, paneOf(flow.RoleLead))

		wantCopilot(t, paneFor(t, env), flow.RoleLead, execed{
			path: copilot,
			args: copilotLine(flow.RoleLead, pluginIn(t, cache), session.String(),
				leadsBrief(store)),
			env: copilotEnv(),
		})
	})
}

// wantCarryOn checks that baton pane carried on at once, with no output, and gave Env.Exec the
// command line, and no other.
func wantCarryOn(t *testing.T, got ran, want execed) {
	t.Helper()

	if got.result != (result{}) {
		t.Errorf("got %+v; want exit code 0 and no output", got.result)
	}

	wantExec(t, got.execs, want)
}

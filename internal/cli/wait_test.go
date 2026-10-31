package cli_test

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The tests run baton wait in synctest's bubbles, where time is fake: the second between two
// looks at the feature passes at once, and so does the hour after which waitAs stops a wait.

// TestWaitPicksUpTheWork has the lead pick up the orchestrator's brief from the linked worktree,
// then refuses its next wait, since no work can come to the lead before it hands over.
func TestWaitPicksUpTheWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		wantWait(t, waitAs(t, linked, flow.RoleLead),
			"lead: round 0 of 3, from orchestrator.", notePath(store, "01-orchestrator-to-lead.md"))
		wantWork(t, store, state.Work{
			Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator, Picked: true,
		})
		wantJournal(t, store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"pickup lead  ", ""},
		})

		before := snapshot(t, store.Dir)

		got, want := waitAs(t, mainDir, flow.RoleLead), result{
			code: 1,
			stderr: "baton wait: lead holds demo-1, with no work left to pick up: " +
				"hand it over before waiting\n",
		}
		if got != want {
			t.Errorf("the next wait: got %+v; want %+v", got, want)
		}

		if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
			t.Errorf("baton's folder changed to %v", after)
		}
	})
}

// TestWaitWaitsForTheRolesWork has the engineer wait while the lead picks up its own work, and
// pick up the lead's note within a second of the handover.
func TestWaitWaitsForTheRolesWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		engineer := make(chan result, 1)
		go func() { engineer <- waitAs(t, linked, flow.RoleEngineer) }()

		time.Sleep(time.Minute)

		if got := waitAs(t, mainDir, flow.RoleLead); got.code != 0 {
			t.Fatalf("the lead's wait: %+v", got)
		}

		time.Sleep(time.Minute)
		synctest.Wait()

		select {
		case got := <-engineer:
			t.Fatalf("the engineer's wait ended before its work came: %+v", got)
		default:
		}

		handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

		handedOver := time.Now()

		wantWait(t, <-engineer,
			"engineer: round 1 of 3, from lead.", notePath(store, "02-lead-to-engineer.md"))

		if waited := time.Since(handedOver); waited > time.Second {
			t.Errorf("the engineer picked up its work %v after the handover; want a second", waited)
		}

		wantJournal(t, store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"pickup lead  ", ""},
			{"handover lead  02-lead-to-engineer.md", "From the lead.\n"},
			{"pickup engineer  ", ""},
		})
	})
}

// TestWaitTellsTheVerdict has the lead pick up the reviewer's approval in an extra round.
func TestWaitTellsTheVerdict(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, flow.State{
			Holder: flow.RoleReviewer, Status: flow.StatusRunning, Round: 4, Cap: 3, Extra: 1,
		})

		handOver(t, mainDir, flow.RoleReviewer, flow.VerdictApprove)

		wantWait(t, waitAs(t, mainDir, flow.RoleLead),
			"lead: round 4 of 4, from reviewer, verdict approve.",
			notePath(store, "02-reviewer-to-lead.md"))
		wantWork(t, store, state.Work{
			Note:    "02-reviewer-to-lead.md",
			From:    flow.RoleReviewer,
			Verdict: flow.VerdictApprove,
			Picked:  true,
		})
	})
}

// TestWaitGivesTheWorkToOneWait has two waits of the engineer race for its work.
func TestWaitGivesTheWorkToOneWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		waits := make(chan result, 2)

		for range 2 {
			go func() { waits <- waitAs(t, linked, flow.RoleEngineer) }()
		}

		time.Sleep(time.Minute)

		handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

		got := []result{<-waits, <-waits}
		slices.SortFunc(got, func(a, b result) int { return a.code - b.code })

		wantWait(t, got[0],
			"engineer: round 1 of 3, from lead.", notePath(store, "02-lead-to-engineer.md"))

		refused := result{
			code: 1,
			stderr: "baton wait: engineer holds demo-1, with no work left to pick up: " +
				"hand it over before waiting\n",
		}
		if got[1] != refused {
			t.Errorf("the other wait: got %+v; want %+v", got[1], refused)
		}
	})
}

// waitAs runs baton wait in the role's pane, from the folder, in a bubble.
func waitAs(t *testing.T, dir string, role flow.Role) result {
	t.Helper()

	return runFor(t, dir, paneOf(role), "wait")
}

// runFor runs baton like run, in a bubble, and stops it after an hour of the bubble's time, so a
// wait that should have ended fails the test at once.
func runFor(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()

	ctx, stop := context.WithTimeout(t.Context(), time.Hour)
	defer stop()

	return runUntil(ctx, dir, env, args...)
}

// handOver hands the feature over in the role's pane, from the folder, with the verdict, if any,
// and a note from the role.
func handOver(t *testing.T, dir string, role flow.Role, verdict flow.Verdict) {
	t.Helper()

	args := []string{"handover", "--note", write(t, dir, "note.md", "From the "+string(role)+".\n")}
	if verdict != flow.VerdictNone {
		args = append(args, "--verdict", string(verdict))
	}

	if got := run(t, dir, paneOf(role), args...); got.code != 0 {
		t.Fatalf("the %s's handover: %+v", role, got)
	}
}

// wantWait checks that baton wait picked up the work, as the role in the round, from the note.
func wantWait(t *testing.T, got result, work, note string) {
	t.Helper()

	want := result{stdout: "Picked up demo-1 as " + work + "\nNote: " + note + "\n"}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

// wantWork checks the holder's work in the feature's file.
func wantWork(t *testing.T, store state.Store, want state.Work) {
	t.Helper()

	feature, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	if feature.Work != want {
		t.Errorf("work: got %+v; want %+v", feature.Work, want)
	}
}

// notePath returns the path of the feature's note with the name.
func notePath(store state.Store, name string) string {
	return filepath.Join(store.Dir, "features", slug, "notes", name)
}

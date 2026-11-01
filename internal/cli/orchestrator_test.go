package cli_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// noneRuns is how baton wait refuses the orchestrator when no work can come to it.
const noneRuns = "baton wait: no feature is running, so no work can come to the orchestrator\n"

// TestOrchestratorPicksUpItsWorkInAnyFeature has the orchestrator wait while three features run,
// then pick up, in the order of their slugs, the work of one that the lead reports ready and of
// one that the reviewer escalates. The third runs on, so the next wait waits for it.
func TestOrchestratorPicksUpItsWorkInAnyFeature(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, flow.State{
			Holder: flow.RoleLead, Status: flow.StatusRunning, Round: 1, Cap: 3,
		})
		add(t, store, state.Feature{Slug: "demo-2", Flow: flow.State{
			Holder: flow.RoleReviewer, Status: flow.StatusRunning, Round: 3, Cap: 3,
		}})
		add(t, store, state.Feature{Slug: "demo-3", Flow: fresh()})

		waited := make(chan result, 1)
		go func() { waited <- runFor(t, linked, orchestratorPane(), "wait") }()

		time.Sleep(time.Minute)
		synctest.Wait()

		select {
		case got := <-waited:
			t.Fatalf("the wait ended before any work came: %+v", got)
		default:
		}

		reviewer := map[string]string{"BATON_ROLE": "reviewer", "BATON_FEATURE": "demo-2"}
		if got := run(t, mainDir, reviewer, "handover", "--verdict", "reject",
			"--note", write(t, mainDir, "reject.md", "Still broken.\n")); got.code != 0 {
			t.Fatalf("the reviewer's handover: %+v", got)
		}

		handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

		handedOver := time.Now()

		wantWait(t, <-waited, "orchestrator: ready, round 1 of 3, from lead.",
			notePath(store, "02-lead-to-orchestrator.md"))

		if waited := time.Since(handedOver); waited > time.Second {
			t.Errorf("the orchestrator picked up its work %v after the handover", waited)
		}

		got, want := runFor(t, mainDir, orchestratorPane(), "wait"), result{
			stdout: "Picked up demo-2 as orchestrator: escalated, round 3 of 3, " +
				"from reviewer, verdict reject.\nNote: " +
				filepath.Join(store.Dir, "features", "demo-2", "notes",
					"02-reviewer-to-orchestrator.md") + "\n",
		}
		if got != want {
			t.Errorf("the next wait: got %+v; want %+v", got, want)
		}

		ctx, stop := context.WithTimeout(t.Context(), time.Minute)
		defer stop()

		got, want = runUntil(ctx, mainDir, orchestratorPane(), "wait"), result{
			code: 1, stderr: "baton wait: stopped waiting: context deadline exceeded\n",
		}
		if got != want {
			t.Errorf("the wait while demo-3 runs: got %+v; want %+v", got, want)
		}

		wantJournal(t, store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"handover lead  02-lead-to-orchestrator.md", "From the lead.\n"},
			{"pickup orchestrator  ", ""},
		})
	})
}

// TestOrchestratorsWorkGoesToOneWait has two waits of the orchestrator race for the work of the
// one feature. The other wait then refuses, since no feature runs.
func TestOrchestratorsWorkGoesToOneWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, flow.State{
			Holder: flow.RoleLead, Status: flow.StatusRunning, Round: 1, Cap: 3,
		})

		waits := make(chan result, 2)

		for range 2 {
			go func() { waits <- runFor(t, linked, orchestratorPane(), "wait") }()
		}

		time.Sleep(time.Minute)

		handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

		got := []result{<-waits, <-waits}
		slices.SortFunc(got, func(a, b result) int { return a.code - b.code })

		wantWait(t, got[0], "orchestrator: ready, round 1 of 3, from lead.",
			notePath(store, "02-lead-to-orchestrator.md"))

		if refused := (result{code: 1, stderr: noneRuns}); got[1] != refused {
			t.Errorf("the other wait: got %+v; want %+v", got[1], refused)
		}
	})
}

// TestOrchestratorHandsFeaturesBack hands an escalated feature to the engineer for one more round,
// and a ready one to the lead with a new brief, which starts the rounds again. The next role picks
// up the orchestrator's note.
func TestOrchestratorHandsFeaturesBack(t *testing.T) {
	cases := []struct {
		name    string
		now     flow.State   // Where the feature stands before the handover to the orchestrator.
		verdict flow.Verdict // The verdict in that handover, if any.
		to      flow.Role
		handed  string // What the handover prints, after "Handed demo-1 over to ".
		picked  string // What the next role's wait prints, after "Picked up demo-1 as ".
	}{
		{
			"an escalated feature to the engineer",
			flow.State{Holder: flow.RoleReviewer, Status: flow.StatusRunning, Round: 3, Cap: 3},
			flow.VerdictReject,
			flow.RoleEngineer,
			"engineer: running, round 4 of 4.",
			"engineer: round 4 of 4, from orchestrator.",
		},
		{
			"a ready feature to the lead",
			flow.State{Holder: flow.RoleLead, Status: flow.StatusRunning, Round: 2, Cap: 3},
			flow.VerdictNone,
			flow.RoleLead,
			"lead: running, round 0 of 3.",
			"lead: round 0 of 3, from orchestrator.",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mainDir, linked := repos(t)
				store := started(t, mainDir, test.now)
				handOver(t, mainDir, test.now.Holder, test.verdict)
				write(t, linked, "word.md", "The user's word.\n")

				got := run(t, linked, orchestratorPane(),
					"handover", "--to", string(test.to), slug, "--note", "word.md")
				if want := (result{stdout: "Handed demo-1 over to " + test.handed + "\n"}); got != want {
					t.Errorf("got %+v; want %+v", got, want)
				}

				note := "03-orchestrator-to-" + string(test.to) + ".md"
				wantWait(t, waitAs(t, linked, test.to), test.picked, notePath(store, note))

				holder := string(test.now.Holder)
				wantJournal(t, store, []logged{
					{
						"start orchestrator  01-orchestrator-to-" + holder + ".md",
						"Build the demo.\n",
					},
					{
						"handover " + holder + " " + string(test.verdict) + " 02-" + holder +
							"-to-orchestrator.md",
						"From the " + holder + ".\n",
					},
					{"handover orchestrator  " + note, "The user's word.\n"},
					{"pickup " + string(test.to) + "  ", ""},
				})
			})
		})
	}
}

// orchestratorPane returns the environment of the orchestrator's pane, which names no feature.
func orchestratorPane() map[string]string {
	return map[string]string{"BATON_ROLE": string(flow.RoleOrchestrator)}
}

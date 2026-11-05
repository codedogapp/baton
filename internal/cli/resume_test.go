package cli_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"uuid"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestOrchestratorsPaneResumesItsSession starts the orchestrator's pane again and again: each start
// gives copilot the session that baton keeps for the orchestrator, which copilot resumes, until the
// sessionStart hook keeps another one, as when the user starts a new session with /new; the next
// start gives copilot that one (decision 68).
func TestOrchestratorsPaneResumesItsSession(t *testing.T) {
	standIn(t)

	mainDir, linked := repos(t)
	first := orchestratorStarts(t, mainDir)

	if again := orchestratorStarts(t, mainDir); again != first {
		t.Errorf("the pane started again in session %s; want %s, the one it kept", again, first)
	}

	next := uuid.New().String()
	if got := sessionHook(t, linked, orchestratorPane(), sessionStarts(t, linked, next)); got !=
		(result{}) {
		t.Fatalf("the session hook: got %+v; want exit code 0 and no output", got)
	}

	if again := orchestratorStarts(t, mainDir); again != next {
		t.Errorf("the pane started again in session %s; want %s, the hook's", again, next)
	}
}

// orchestratorStarts starts the orchestrator's pane in the main worktree, and returns the ID of
// the session that the pane gave the orchestrator's copilot, which baton keeps.
func orchestratorStarts(t *testing.T, mainDir string) string {
	t.Helper()

	env, _ := inPane(t, mainDir, orchestratorPane())

	got := runExec(t.Context(), env, nil)
	if got.result != (result{}) || len(got.execs) != 1 {
		t.Fatalf("got %+v; want copilot, with no output", got)
	}

	args := got.execs[0].args

	flag := slices.Index(args, "--session-id")
	if flag < 0 || flag+1 == len(args) {
		t.Fatalf("copilot got %q; want a session's ID", args)
	}

	session := args[flag+1]
	if kept := orchestratorSession(t, mainDir); session != kept {
		t.Errorf("copilot got session %s; want %s, the one that baton keeps", session, kept)
	}

	return session
}

// TestOrchestratorsPaneGivesBackWhatItPickedUp starts the orchestrator's pane after its last
// copilot's wait picked up what still waits on the orchestrator: a role's question without its
// answer, or the work of a ready or escalated feature, which waits on the user's word. The pane
// gives it back, so that the next wait picks it up again, and logs that in the feature's journal.
// It leaves alone what no wait of the orchestrator picked up, a question with its answer, and the
// work of a feature that the user has delivered or closed (decision 68).
func TestOrchestratorsPaneGivesBackWhatItPickedUp(t *testing.T) {
	standIn(t)

	held := func(status flow.Status) flow.State {
		return flow.State{Holder: flow.RoleOrchestrator, Status: status, Round: 3, Cap: 3}
	}
	asked := state.Question{From: flow.RoleLead, Text: question}
	askedAndPicked := func(feature *state.Feature) {
		feature.Question, feature.Question.Picked = asked, true
	}
	workPicked := func(feature *state.Feature) { feature.Work.Picked = true }

	cases := []struct {
		name   string
		now    flow.State
		change func(feature *state.Feature) // The pickup, or the hand edit, before the pane.
		back   func(feature *state.Feature) // What the pane gives back, if anything.
	}{
		{
			name: "a question that the orchestrator picked up", now: fresh(),
			change: askedAndPicked,
			back:   func(feature *state.Feature) { feature.Question.Picked = false },
		},
		{
			name: "a ready feature's work that it picked up", now: held(flow.StatusReady),
			change: workPicked,
			back:   func(feature *state.Feature) { feature.Work.Picked = false },
		},
		{
			name: "an escalated feature's work that it picked up", now: held(flow.StatusEscalated),
			change: workPicked,
			back:   func(feature *state.Feature) { feature.Work.Picked = false },
		},
		{
			name: "a question that it hasn't picked up", now: fresh(),
			change: func(feature *state.Feature) { feature.Question = asked },
		},
		{
			name: "a question with its answer", now: fresh(),
			change: func(feature *state.Feature) {
				askedAndPicked(feature)
				feature.Question.Answer = "demo.txt"
			},
		},
		{
			name: "a ready feature's work that it hasn't picked up", now: held(flow.StatusReady),
			change: func(*state.Feature) {},
		},
		{name: "the work that the lead picked up", now: fresh(), change: workPicked},
		{
			name: "a delivered feature's work", now: held(flow.StatusDelivered),
			change: workPicked,
		},
		{name: "a closed feature's work", now: held(flow.StatusClosed), change: workPicked},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mainDir, _ := repos(t)
			store := started(t, mainDir, test.now)
			edit(t, store, test.change)

			want := readFeature(t, store)
			journal := []logged{
				{
					"start orchestrator  01-orchestrator-to-" + string(test.now.Holder) + ".md",
					"Build the demo.\n",
				},
				{"edit orchestrator  ", ""},
			}

			if test.back != nil {
				test.back(&want)

				journal = append(journal, logged{"resume orchestrator  ", ""})
			}

			orchestratorStarts(t, mainDir)

			if got := readFeature(t, store); got != want {
				t.Errorf("the feature is %+v; want %+v", got, want)
			}

			wantJournal(t, store, journal)
		})
	}
}

// readFeature returns demo-1 as its file holds it.
func readFeature(t *testing.T, store state.Store) state.Feature {
	t.Helper()

	feature, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	return feature
}

// TestOrchestratorsNextWaitsPickUpWhatItsPaneGaveBack has the orchestrator's waits pick up the
// lead's question in demo-1 and the lead's final report in demo-2, then starts the orchestrator's
// pane again, as after its copilot ended: its next waits pick both up again, as the first did.
func TestOrchestratorsNextWaitsPickUpWhatItsPaneGaveBack(t *testing.T) {
	standIn(t)

	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())
		add(t, store, state.Feature{Slug: "demo-2", Flow: flow.State{
			Holder: flow.RoleOrchestrator, Status: flow.StatusReady, Round: 1, Cap: 3,
		}})

		waitAs(t, linked, flow.RoleLead)
		askAMinute(t, linked, question)

		waits := func() []result {
			return []result{
				runFor(t, mainDir, orchestratorPane(), "wait"),
				runFor(t, mainDir, orchestratorPane(), "wait"),
			}
		}

		first := waits()
		if !strings.HasPrefix(first[0].stdout, "Picked up the lead's question in demo-1:\n") ||
			!strings.HasPrefix(first[1].stdout, "Picked up demo-2 as orchestrator: ready,") {
			t.Fatalf("the first waits: got %+v; want the question, then the ready feature", first)
		}

		orchestratorStarts(t, mainDir)

		if again := waits(); !slices.Equal(again, first) {
			t.Errorf("the next waits: got %+v; want %+v", again, first)
		}
	})
}

// TestOrchestratorsPaneRefusesADamagedFolder refuses to start the orchestrator's copilot when it
// can't read the orchestrator's file or a feature's, or keep a new session, and then gives nothing
// back (risk 19).
func TestOrchestratorsPaneRefusesADamagedFolder(t *testing.T) {
	standIn(t)

	// Each damages baton's folder, and returns the start of the pane's reason.
	damages := map[string]func(t *testing.T, baton string) string{
		"the orchestrator's file": func(t *testing.T, baton string) string {
			t.Helper()

			return "orchestrator file " + damage(t, baton, "orchestrator.json") + newerFormat + "\n"
		},
		"a feature's file": func(t *testing.T, baton string) string {
			t.Helper()

			return "feature file " + damage(t, baton, "features", "demo-2", "feature.json") +
				newerFormat + "\n"
		},
		"a feature that it can't give back": func(t *testing.T, baton string) string {
			t.Helper()

			// A folder in the place of the file that baton writes first, which it can't remove.
			mkdir(t, filepath.Join(baton, "features", slug, "feature.json.tmp", "full"))

			return "write feature " + slug + ": "
		},
		"a session that it can't keep": func(t *testing.T, baton string) string {
			t.Helper()

			// A folder in the place of the file that baton writes first, which it can't remove.
			mkdir(t, filepath.Join(baton, "orchestrator.json.tmp", "full"))

			return "write the orchestrator's file: "
		},
	}

	for name, damaging := range damages {
		t.Run(name, func(t *testing.T) {
			mainDir, _ := repos(t)
			store := started(t, mainDir, fresh())
			edit(t, store, func(feature *state.Feature) {
				feature.Question = state.Question{From: flow.RoleLead, Text: question, Picked: true}
			})

			reason := damaging(t, store.Dir)
			before := readFeature(t, store)
			env, _ := inPane(t, mainDir, orchestratorPane())

			wantRefused(t, runExec(t.Context(), env, nil), "", reason)

			if after := readFeature(t, store); after != before {
				t.Errorf("the feature is %+v; want it unchanged, %+v", after, before)
			}
		})
	}
}

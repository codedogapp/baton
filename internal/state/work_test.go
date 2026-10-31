package state_test

import (
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

func TestCreateRefusesAFeatureWithoutItsWork(t *testing.T) {
	store := state.Store{Dir: t.TempDir()}
	entry := briefed()
	entry.Note = nil

	wantError(t, store.Create(started(t), entry), "new feature demo-1: want a note, lead's work")
	wantNothingIn(t, store.Dir)
}

func TestUpdateRefusesAMoveWithoutItsWork(t *testing.T) {
	store := created(t)
	before := read(t, featureFile(store))

	got, err := store.Update(slug, logged(), hand(flow.Handover{From: flow.RoleLead}))
	wantError(t, err, "change feature demo-1: want a note, engineer's work")

	if got != (state.Feature{}) || read(t, featureFile(store)) != before ||
		len(events(t, store)) != 1 {
		t.Errorf("Update = %+v and wrote a move without its note", got)
	}
}

func TestEachNoteBecomesTheHoldersWork(t *testing.T) {
	store := created(t)
	lead, engineer := flow.RoleLead, flow.RoleEngineer
	pickup := state.Entry{Event: state.Event{Kind: state.EventPickup, Role: lead}}
	pick := func(feature *state.Feature) error {
		feature.Work.Picked = true
		return nil
	}

	steps := []struct {
		entry  state.Entry
		change func(*state.Feature) error
		want   state.Work
	}{
		{
			pickup,
			pick,
			state.Work{
				Note:   "01-orchestrator-to-lead.md",
				From:   flow.RoleOrchestrator,
				Picked: true,
			},
		},
		{
			noted(lead, ""),
			hand(flow.Handover{From: lead}),
			state.Work{Note: "02-lead-to-engineer.md", From: lead},
		},
		{logged(), keep, state.Work{Note: "02-lead-to-engineer.md", From: lead}},
		{
			noted(engineer, ""),
			hand(flow.Handover{From: engineer}),
			state.Work{Note: "03-engineer-to-qa.md", From: engineer},
		},
		{
			noted(flow.RoleQA, flow.VerdictReject),
			hand(flow.Handover{From: flow.RoleQA, Verdict: flow.VerdictReject}),
			state.Work{
				Note:    "04-qa-to-engineer.md",
				From:    flow.RoleQA,
				Verdict: flow.VerdictReject,
			},
		},
	}

	for _, step := range steps {
		got, err := store.Update(slug, step.entry, step.change)
		if err != nil || got.Work != step.want {
			t.Fatalf("Update = %+v, %v; want the work %+v", got.Work, err, step.want)
		}

		if got, err := store.Read(slug); err != nil || got.Work != step.want {
			t.Errorf("Read = %+v, %v; want the work %+v", got.Work, err, step.want)
		}
	}
}

func TestNotePathIsTheNotesFile(t *testing.T) {
	store := created(t)

	if got := read(t, store.NotePath(slug, "01-orchestrator-to-lead.md")); got != brief {
		t.Errorf("the brief's path holds %q, want %q", got, brief)
	}
}

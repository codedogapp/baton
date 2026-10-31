package flow_test

import (
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

// moves returns the handovers that the tests make, by name.
func moves() map[string]flow.Handover {
	return map[string]flow.Handover{
		"lead":                     {From: flow.RoleLead},
		"engineer":                 {From: flow.RoleEngineer},
		"qa approves":              {From: flow.RoleQA, Verdict: flow.VerdictApprove},
		"qa rejects":               {From: flow.RoleQA, Verdict: flow.VerdictReject},
		"reviewer approves":        {From: flow.RoleReviewer, Verdict: flow.VerdictApprove},
		"reviewer rejects":         {From: flow.RoleReviewer, Verdict: flow.VerdictReject},
		"orchestrator to lead":     {From: flow.RoleOrchestrator, To: flow.RoleLead},
		"orchestrator to engineer": {From: flow.RoleOrchestrator, To: flow.RoleEngineer},
	}
}

// walk starts a feature with the round cap and makes the named handovers in order.
func walk(t *testing.T, roundCap int, names ...string) flow.State {
	t.Helper()

	state, err := flow.Start(roundCap)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range names {
		handover, ok := moves()[name]
		if !ok {
			t.Fatalf("no handover named %q", name)
		}

		if state, err = state.Next(handover); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	return state
}

// approved returns the handovers of a first round that QA and the reviewer approve.
func approved() []string {
	return []string{"lead", "engineer", "qa approves", "reviewer approves"}
}

// escalated returns the handovers of three rounds with a cap of 3, each rejected.
func escalated() []string {
	return []string{
		"lead", "engineer", "qa rejects", "engineer", "qa approves", "reviewer rejects",
		"engineer", "qa rejects",
	}
}

func TestStartGivesTheLeadTheBrief(t *testing.T) {
	got := walk(t, 3)

	if want := (flow.State{Holder: flow.RoleLead, Status: flow.StatusRunning, Cap: 3}); got != want {
		t.Errorf("Start(3) = %+v, want %+v", got, want)
	}
}

func TestStartNeedsARound(t *testing.T) {
	for _, roundCap := range []int{0, -1} {
		if _, err := flow.Start(roundCap); err == nil || !strings.Contains(err.Error(), "want 1") {
			t.Errorf("Start(%d) error = %v, want one that asks for a round", roundCap, err)
		}
	}
}

func TestNextFollowsTheFlow(t *testing.T) {
	tests := []struct {
		name     string
		roundCap int
		names    []string
		want     flow.State
	}{
		{
			"the lead briefs the engineer",
			3,
			approved()[:1],
			state(flow.RoleEngineer, flow.StatusRunning, 1, 0),
		},
		{
			"the engineer hands over to QA",
			3,
			approved()[:2],
			state(flow.RoleQA, flow.StatusRunning, 1, 0),
		},
		{
			"QA approves to the reviewer",
			3,
			approved()[:3],
			state(flow.RoleReviewer, flow.StatusRunning, 1, 0),
		},
		{
			"the reviewer approves to the lead",
			3,
			approved(),
			state(flow.RoleLead, flow.StatusRunning, 1, 0),
		},
		{
			"the lead's final report makes the feature ready", 3, append(approved(), "lead"),
			state(flow.RoleOrchestrator, flow.StatusReady, 1, 0),
		},
		{
			"QA rejects to the engineer", 3, escalated()[:3],
			state(flow.RoleEngineer, flow.StatusRunning, 2, 0),
		},
		{
			"the reviewer rejects to the engineer", 3, escalated()[:6],
			state(flow.RoleEngineer, flow.StatusRunning, 3, 0),
		},
		{
			"a rejection after the last round escalates", 3, escalated(),
			state(flow.RoleOrchestrator, flow.StatusEscalated, 3, 0),
		},
		{
			"the user's word buys one more round",
			3,
			append(escalated(), "orchestrator to engineer"),
			state(flow.RoleEngineer, flow.StatusRunning, 4, 1),
		},
		{
			"a rejection in the extra round escalates again", 3,
			append(escalated(), "orchestrator to engineer", "engineer", "qa rejects"),
			state(flow.RoleOrchestrator, flow.StatusEscalated, 4, 1),
		},
		{
			"a new brief starts the rounds again",
			3,
			append(
				escalated(),
				"orchestrator to engineer",
				"engineer",
				"qa rejects",
				"orchestrator to lead",
			),
			state(flow.RoleLead, flow.StatusRunning, 0, 0),
		},
		{
			"changes after the final report go to the lead", 3,
			append(approved(), "lead", "orchestrator to lead", "lead"),
			state(flow.RoleEngineer, flow.StatusRunning, 1, 0),
		},
		{
			"a cap of one escalates the first rejection",
			1,
			escalated()[:3],
			flow.State{
				Holder: flow.RoleOrchestrator,
				Status: flow.StatusEscalated,
				Round:  1,
				Cap:    1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := walk(t, test.roundCap, test.names...); got != test.want {
				t.Errorf("after %q: %+v, want %+v", test.names, got, test.want)
			}
		})
	}
}

// state returns a state with a round cap of 3.
func state(holder flow.Role, status flow.Status, round, extra int) flow.State {
	return flow.State{Holder: holder, Status: status, Round: round, Cap: 3, Extra: extra}
}

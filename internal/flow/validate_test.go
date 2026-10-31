package flow_test

import (
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

func TestValidateAcceptsExactlyTheStatesOfTheFlow(t *testing.T) {
	const maxCap, maxExtra = 3, 2

	made := madeStates(t, maxCap, maxExtra)
	roles := roles()
	statuses := []flow.Status{
		"",
		"done",
		flow.StatusRunning,
		flow.StatusEscalated,
		flow.StatusReady,
	}

	states := []flow.State{{}}
	states = vary(states, len(roles), func(s *flow.State, i int) { s.Holder = roles[i] })
	states = vary(states, len(statuses), func(s *flow.State, i int) { s.Status = statuses[i] })
	states = vary(states, maxCap+maxExtra+3, func(s *flow.State, i int) { s.Round = i - 1 })
	states = vary(states, maxCap+1, func(s *flow.State, i int) { s.Cap = i })
	states = vary(states, maxExtra+2, func(s *flow.State, i int) { s.Extra = i - 1 })

	for _, state := range states {
		if err := state.Validate(); (err == nil) != made[state] {
			t.Errorf("%+v.Validate() = %v, but the flow makes it: %t", state, err, made[state])
		}
	}
}

// roles returns the roles that the tests try: every role, none, and an unknown one.
func roles() []flow.Role {
	return []flow.Role{
		"", "boss", flow.RoleOrchestrator, flow.RoleLead, flow.RoleEngineer, flow.RoleQA,
		flow.RoleReviewer,
	}
}

// madeStates returns the states that Start and Next make, up to the caps and extra rounds given,
// by trying every handover from every state found.
func madeStates(t *testing.T, maxCap, maxExtra int) map[flow.State]bool {
	t.Helper()

	var found []flow.State

	for roundCap := 1; roundCap <= maxCap; roundCap++ {
		start, err := flow.Start(roundCap)
		if err != nil {
			t.Fatal(err)
		}

		found = append(found, start)
	}

	roles := roles()
	verdicts := []flow.Verdict{flow.VerdictNone, flow.VerdictApprove, flow.VerdictReject, "maybe"}

	handovers := []flow.Handover{{}}
	handovers = vary(handovers, len(roles), func(h *flow.Handover, i int) { h.From = roles[i] })
	handovers = vary(handovers, len(roles), func(h *flow.Handover, i int) { h.To = roles[i] })
	handovers = vary(handovers, len(verdicts), func(h *flow.Handover, i int) {
		h.Verdict = verdicts[i]
	})

	made := map[flow.State]bool{}

	for len(found) > 0 {
		state := found[0]
		found = found[1:]

		if made[state] || state.Extra > maxExtra {
			continue
		}

		made[state] = true

		for _, handover := range handovers {
			if next, err := state.Next(handover); err == nil {
				found = append(found, next)
			}
		}
	}

	return made
}

// vary returns n copies of each value, the i-th one changed by set(value, i).
func vary[T any](values []T, n int, set func(*T, int)) []T {
	varied := make([]T, 0, len(values)*n)

	for _, value := range values {
		for i := range n {
			set(&value, i)
			varied = append(varied, value)
		}
	}

	return varied
}

func TestValidateRefusesStatesTheFlowNeverMakes(t *testing.T) {
	tests := []struct {
		name  string
		state flow.State
		want  string
	}{
		{"the zero state", flow.State{}, `unknown holder ""`},
		{"an unknown holder", state("boss", flow.StatusRunning, 1, 0), `unknown holder "boss"`},
		{"an unknown status", state(flow.RoleQA, "done", 1, 0), `unknown status "done"`},
		{
			"the orchestrator while the roles work",
			state(flow.RoleOrchestrator, flow.StatusRunning, 1, 0),
			"orchestrator can't hold a feature that is running",
		},
		{
			"a role once the feature is ready",
			state(flow.RoleLead, flow.StatusReady, 1, 0),
			"lead can't hold a feature that is ready",
		},
		{
			"the engineer before the first round",
			state(flow.RoleEngineer, flow.StatusRunning, 0, 0),
			"engineer can't hold a feature before its first round",
		},
		{
			"no round cap",
			flow.State{Holder: flow.RoleLead, Status: flow.StatusRunning},
			"round cap 0: want 1 or more",
		},
		{
			"fewer than no extra rounds",
			state(flow.RoleQA, flow.StatusRunning, 1, -1),
			"extra rounds -1: want 0 or more",
		},
		{
			"a negative round",
			state(flow.RoleQA, flow.StatusRunning, -1, 0),
			"round -1: want 0 to 3",
		},
		{
			"a round past the cap",
			state(flow.RoleQA, flow.StatusRunning, 4, 0),
			"round 4: want 0 to 3",
		},
		{
			"an escalation before the last round",
			state(flow.RoleOrchestrator, flow.StatusEscalated, 2, 0),
			"round 2: want 3, the last round, in an escalation",
		},
		{
			"extra rounds before the last round",
			state(flow.RoleEngineer, flow.StatusRunning, 3, 1),
			"round 3: want 4, the last round, with extra rounds",
		},
		{
			"extra rounds before the first round",
			state(flow.RoleLead, flow.StatusRunning, 0, 1),
			"round 0: want 4, the last round, with extra rounds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.state.Validate(); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Errorf("%+v.Validate() = %v, want an error with %q", test.state, err, test.want)
			}
		})
	}
}

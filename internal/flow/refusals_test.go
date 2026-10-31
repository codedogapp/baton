package flow_test

import (
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

func TestNextRefusesMovesOutsideTheFlow(t *testing.T) {
	ready := append(approved(), "lead")

	tests := []struct {
		name     string
		names    []string
		handover flow.Handover
		want     string
	}{
		{
			"a role that doesn't hold the feature", nil,
			flow.Handover{From: flow.RoleEngineer},
			"lead holds the feature, not engineer",
		},
		{
			"the orchestrator while a role holds the feature", approved()[:1],
			flow.Handover{From: flow.RoleOrchestrator, To: flow.RoleLead},
			"engineer holds the feature, not orchestrator",
		},
		{
			"a role that picks the next role", nil,
			flow.Handover{From: flow.RoleLead, To: flow.RoleQA},
			"lead can't pick the next role",
		},
		{
			"QA without a verdict", approved()[:2],
			flow.Handover{From: flow.RoleQA},
			"qa hands over with a verdict: approve or reject",
		},
		{
			"the reviewer with an unknown verdict", approved()[:3],
			flow.Handover{From: flow.RoleReviewer, Verdict: "maybe"},
			"reviewer hands over with a verdict: approve or reject",
		},
		{
			"the engineer with a verdict", approved()[:1],
			flow.Handover{From: flow.RoleEngineer, Verdict: flow.VerdictApprove},
			"engineer hands over without a verdict",
		},
		{
			"the lead with a rejection", nil,
			flow.Handover{From: flow.RoleLead, Verdict: flow.VerdictReject},
			"lead hands over without a verdict",
		},
		{
			"a ready feature to the engineer", ready,
			flow.Handover{From: flow.RoleOrchestrator, To: flow.RoleEngineer},
			`can't hand a feature that is ready to "engineer"`,
		},
		{
			"an escalated feature to QA", escalated(),
			flow.Handover{From: flow.RoleOrchestrator, To: flow.RoleQA},
			`can't hand a feature that is escalated to "qa"`,
		},
		{
			"an escalated feature to nobody", escalated(),
			flow.Handover{From: flow.RoleOrchestrator},
			`can't hand a feature that is escalated to ""`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := walk(t, 3, test.names...)

			_, err := before.Next(test.handover)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("Next(%+v) after %q: error = %v, want one with %q",
					test.handover, test.names, err, test.want)
			}
		})
	}
}

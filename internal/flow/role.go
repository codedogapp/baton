// Package flow decides who works next on a feature: the roles, the handovers between them, the
// rounds of QA and review, and the escalation to the user.
package flow

// Role is the part that an agent plays.
type Role string

// The roles. The user talks to the orchestrator; the other four work on one feature.
const (
	RoleOrchestrator Role = "orchestrator"
	RoleLead         Role = "lead"
	RoleEngineer     Role = "engineer"
	RoleQA           Role = "qa"
	RoleReviewer     Role = "reviewer"
)

// known reports whether the role is one of the roles above.
func (r Role) known() bool {
	switch r {
	case RoleOrchestrator, RoleLead, RoleEngineer, RoleQA, RoleReviewer:
		return true
	default:
		return false
	}
}

// judges reports whether the role hands over with a verdict on the engineer's work.
func (r Role) judges() bool {
	return r == RoleQA || r == RoleReviewer
}

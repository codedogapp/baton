package flow

import "fmt"

// Status says whether the roles work on a feature or the orchestrator holds it, and why.
type Status string

// The statuses.
const (
	StatusRunning   Status = "running"   // A role other than the orchestrator holds the feature.
	StatusEscalated Status = "escalated" // A rejection came after the last round; the user decides.
	StatusReady     Status = "ready"     // The lead's final report is in; the user decides.
)

// State is where a feature stands in the flow.
type State struct {
	Holder Role   // The role whose turn it is; only the holder hands over.
	Status Status // Whether the roles work on the feature, or why the orchestrator holds it.
	Round  int    // The engineer's passes since the last brief, the current one included.
	Cap    int    // The passes that a brief gets; a rejection after the last one escalates.
	Extra  int    // The passes that the user added after escalations, since the last brief.
}

// Start returns a new feature's state: the lead holds the orchestrator's brief.
func Start(roundCap int) (State, error) {
	if roundCap < 1 {
		return State{}, fmt.Errorf("round cap %d: want 1 or more", roundCap)
	}

	return State{Holder: RoleLead, Status: StatusRunning, Cap: roundCap}, nil
}

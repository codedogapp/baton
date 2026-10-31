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

// known reports whether the status is one of the statuses above.
func (s Status) known() bool {
	switch s {
	case StatusRunning, StatusEscalated, StatusReady:
		return true
	default:
		return false
	}
}

// State is where a feature stands in the flow.
type State struct {
	// Holder is the role whose turn it is; only the holder hands over.
	Holder Role `json:"holder"`
	// Status says whether the roles work on the feature, or why the orchestrator holds it.
	Status Status `json:"status"`
	// Round counts the engineer's passes since the last brief, the current one included.
	Round int `json:"round"`
	// Cap is the passes that a brief gets; a rejection after the last one escalates.
	Cap int `json:"cap"`
	// Extra is the passes that the user added after escalations, since the last brief.
	Extra int `json:"extra"`
}

// Start returns a new feature's state: the lead holds the orchestrator's brief.
func Start(roundCap int) (State, error) {
	state := State{Holder: RoleLead, Status: StatusRunning, Cap: roundCap}
	if err := state.Validate(); err != nil {
		return State{}, err
	}

	return state, nil
}

// Validate returns an error if Start and Next can't make the state, as when it comes from a
// damaged file.
func (s State) Validate() error {
	switch {
	case !s.Holder.known():
		return fmt.Errorf("unknown holder %q", s.Holder)
	case !s.Status.known():
		return fmt.Errorf("unknown status %q", s.Status)
	case (s.Holder == RoleOrchestrator) == (s.Status == StatusRunning):
		return fmt.Errorf("%s can't hold a feature that is %s", s.Holder, s.Status)
	case s.Round == 0 && s.Holder != RoleLead:
		return fmt.Errorf("%s can't hold a feature before its first round", s.Holder)
	default:
		return s.checkRounds()
	}
}

// checkRounds returns an error if the round, the cap or the extra rounds are out of range. Only
// an escalation leads to extra rounds, one at a time, so the round is then the last one, as it is
// in an escalation.
func (s State) checkRounds() error {
	last := s.Cap + s.Extra

	switch {
	case s.Cap < 1:
		return fmt.Errorf("round cap %d: want 1 or more", s.Cap)
	case s.Extra < 0:
		return fmt.Errorf("extra rounds %d: want 0 or more", s.Extra)
	case s.Round < 0 || s.Round > last:
		return fmt.Errorf("round %d: want 0 to %d, the cap plus the extra rounds", s.Round, last)
	case s.Status == StatusEscalated && s.Round != last:
		return fmt.Errorf("round %d: want %d, the last round, in an escalation", s.Round, last)
	case s.Extra > 0 && s.Round != last:
		return fmt.Errorf("round %d: want %d, the last round, with extra rounds", s.Round, last)
	default:
		return nil
	}
}

package flow

import "fmt"

// Verdict is QA's or the reviewer's judgement of the engineer's work.
type Verdict string

// The verdicts. The roles other than QA and the reviewer hand over without one.
const (
	VerdictNone    Verdict = ""
	VerdictApprove Verdict = "approve"
	VerdictReject  Verdict = "reject"
)

// Handover is a move that the holder asks for.
type Handover struct {
	From    Role
	To      Role    // The orchestrator's pick; baton picks the next role for the others.
	Verdict Verdict // QA's and the reviewer's verdict.
}

// Next returns the state after the handover, or an error if the flow doesn't allow it.
func (s State) Next(handover Handover) (State, error) {
	if err := s.check(handover); err != nil {
		return State{}, err
	}

	switch {
	case handover.From == RoleOrchestrator:
		return s.handBack(handover.To)
	case handover.Verdict == VerdictReject:
		return s.rejected(), nil
	default:
		return s.passed(handover.From), nil
	}
}

// check returns an error if the handover breaks the rules of who may hand over, and how.
func (s State) check(handover Handover) error {
	from := handover.From

	switch {
	case from != s.Holder:
		return fmt.Errorf("%s holds the feature, not %s", s.Holder, from)
	case from != RoleOrchestrator && handover.To != "":
		return fmt.Errorf("%s can't pick the next role: baton picks it", from)
	case from.judges() && handover.Verdict != VerdictApprove && handover.Verdict != VerdictReject:
		return fmt.Errorf("%s hands over with a verdict: approve or reject", from)
	case !from.judges() && handover.Verdict != VerdictNone:
		return fmt.Errorf("%s hands over without a verdict", from)
	default:
		return nil
	}
}

// passed hands the work on from a role that is done with it, or approves it.
func (s State) passed(from Role) State {
	switch from {
	case RoleLead:
		if s.Round > 0 { // The final report, after the reviewer's approval.
			s.Holder, s.Status = RoleOrchestrator, StatusReady
			return s
		}

		s.Holder, s.Round = RoleEngineer, 1
	case RoleEngineer:
		s.Holder = RoleQA
	case RoleQA:
		s.Holder = RoleReviewer
	default: // The reviewer.
		s.Holder = RoleLead
	}

	return s
}

// rejected gives the work back to the engineer for another round or, after the last round,
// escalates the feature to the orchestrator.
func (s State) rejected() State {
	if s.Round >= s.Cap+s.Extra {
		s.Holder, s.Status = RoleOrchestrator, StatusEscalated
		return s
	}

	s.Holder = RoleEngineer
	s.Round++

	return s
}

// handBack moves the feature from the orchestrator, on the user's word: to the lead with a new
// brief, which starts the rounds again, or, after an escalation, to the engineer for one more
// round.
func (s State) handBack(next Role) (State, error) {
	switch {
	case next == RoleLead:
		return State{Holder: RoleLead, Status: StatusRunning, Cap: s.Cap}, nil
	case next == RoleEngineer && s.Status == StatusEscalated:
		s.Holder, s.Status = RoleEngineer, StatusRunning
		s.Round++
		s.Extra++

		return s, nil
	default:
		return State{}, fmt.Errorf(
			"the orchestrator can't hand a feature that is %s to %q: "+
				"it goes to the lead or, after an escalation, to the engineer", s.Status, next)
	}
}

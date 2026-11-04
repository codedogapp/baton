package flow_test

import (
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

// ready returns the state after the lead's final report, in the first round.
func ready(t *testing.T) flow.State {
	t.Helper()

	return walk(t, 3, append(approved(), "lead")...)
}

func TestDeliverKeepsTheFeatureForTheUser(t *testing.T) {
	got, err := ready(t).Deliver()
	if want := state(flow.RoleOrchestrator, flow.StatusDelivered, 1, 0); err != nil || got != want {
		t.Fatalf("Deliver() = %+v, %v; want %+v", got, err, want)
	}

	// The user can send it back with changes, which start the rounds again.
	back, err := got.Next(moves()["orchestrator to lead"])
	if want := state(flow.RoleLead, flow.StatusRunning, 0, 0); err != nil || back != want {
		t.Errorf("the new brief after a delivery: %+v, %v; want %+v", back, err, want)
	}
}

func TestDeliverRefusesAFeatureThatIsNotReady(t *testing.T) {
	delivered, err := ready(t).Deliver()
	if err != nil {
		t.Fatal(err)
	}

	for _, now := range []flow.State{
		walk(t, 3, approved()...),
		walk(t, 3, escalated()...),
		delivered,
	} {
		got, err := now.Deliver()

		want := "a feature that is " + string(now.Status) + " isn't ready to deliver"
		if err == nil || !strings.HasPrefix(err.Error(), want) || got != (flow.State{}) {
			t.Errorf("%+v.Deliver() = %+v, %v; want an error that starts with %q", now, got,
				err, want)
		}
	}
}

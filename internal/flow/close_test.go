package flow_test

import (
	"cmp"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

func TestCloseGivesAnyFeatureToTheOrchestratorAtItsRound(t *testing.T) {
	delivered, err := ready(t).Deliver()
	if err != nil {
		t.Fatal(err)
	}

	for _, now := range []flow.State{
		walk(t, 3),
		walk(t, 3, "lead", "engineer"),
		walk(t, 3, escalated()...),
		walk(t, 3, append(escalated(), "orchestrator to engineer")...),
		ready(t),
		delivered,
	} {
		want := now
		want.Holder, want.Status = flow.RoleOrchestrator, flow.StatusClosed

		if got, err := now.Close(); err != nil || got != want {
			t.Errorf("%+v.Close() = %+v, %v; want %+v", now, got, err, want)
		}
	}
}

func TestNothingMovesAClosedFeature(t *testing.T) {
	closed, err := ready(t).Close()
	if err != nil {
		t.Fatal(err)
	}

	tries := map[string]func() (flow.State, error){
		"Deliver": closed.Deliver,
		"Close":   closed.Close,
	}
	for name, handover := range moves() {
		tries[name] = func() (flow.State, error) { return closed.Next(handover) }
	}

	wants := map[string]string{
		"Deliver": "the feature is closed, and nothing delivers it",
		"Close":   "the feature is closed already",
	}

	for name, try := range tries {
		want := cmp.Or(wants[name], "the feature is closed, and nothing hands it over again")
		if got, err := try(); err == nil || err.Error() != want || got != (flow.State{}) {
			t.Errorf("%s = %+v, %v; want the error %q", name, got, err, want)
		}
	}
}

package state_test

import (
	"strings"
	"testing"
	"uuid"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestUpdateKeepsTheSessions keeps the ID of the session that a role's pane started, after the
// flow in the feature's file, and none for the roles whose panes started none yet (decision 66).
func TestUpdateKeepsTheSessions(t *testing.T) {
	session := uuid.MustParse("0b8f7c55-3a0e-4c1f-9d6e-1f2a3b4c5d6e")

	for _, role := range []flow.Role{flow.RoleLead, flow.RoleEngineer, flow.RoleQA, flow.RoleReviewer} {
		t.Run(string(role), func(t *testing.T) {
			store := created(t)

			got, err := store.Update(slug, logged(), func(feature *state.Feature) error {
				*feature.Sessions.Of(role) = session

				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			want := "    \"extra\": 0\n  },\n  \"sessions\": {\n    \"" + string(role) + "\": \"" +
				session.String() + "\"\n  },\n  \"work\": {\n"
			if file := read(t, featureFile(store)); !strings.Contains(file, want) {
				t.Errorf("the feature file holds\n%s\nwant it to hold\n%s", file, want)
			}

			if *got.Sessions.Of(role) != session {
				t.Errorf("Update = %+v; want the %s's session %s", got, role, session)
			}

			if read, err := store.Read(slug); err != nil || read != got {
				t.Errorf("Read = %+v, %v; want %+v", read, err, got)
			}
		})
	}
}

// TestSessionsOfTheRoles finds each role's session in its own field, and none for the
// orchestrator.
func TestSessionsOfTheRoles(t *testing.T) {
	var sessions state.Sessions

	for role, want := range map[flow.Role]*uuid.UUID{
		flow.RoleLead: &sessions.Lead, flow.RoleEngineer: &sessions.Engineer,
		flow.RoleQA: &sessions.QA, flow.RoleReviewer: &sessions.Reviewer,
		flow.RoleOrchestrator: nil,
	} {
		if got := sessions.Of(role); got != want {
			t.Errorf("Of(%s) = %p; want %p", role, got, want)
		}
	}
}

package state_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// keep is a change that changes nothing.
func keep(*state.Feature) error {
	return nil
}

// hand returns a change that makes the handover.
func hand(handover flow.Handover) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		next, err := feature.Flow.Next(handover)
		if err != nil {
			return err
		}

		feature.Flow = next

		return nil
	}
}

func TestUpdateWritesTheChange(t *testing.T) {
	store := created(t)

	got, err := store.Update(slug, hand(flow.Handover{From: flow.RoleLead}))
	if err != nil {
		t.Fatal(err)
	}

	want := started(t)
	want.Flow.Holder, want.Flow.Round = flow.RoleEngineer, 1

	if got != want {
		t.Errorf("Update = %+v, want %+v", got, want)
	}

	if got, err := store.Read(slug); err != nil || got != want {
		t.Errorf("Read = %+v, %v; want %+v", got, err, want)
	}
}

func TestUpdateWritesNothingWhenTheChangeFails(t *testing.T) {
	refused := errors.New("refused")

	tests := []struct {
		name   string
		change func(*state.Feature) error
		want   string
	}{
		{"the change fails", func(*state.Feature) error { return refused }, "refused"},
		{
			"the flow refuses the handover",
			hand(flow.Handover{From: flow.RoleQA, Verdict: flow.VerdictApprove}),
			"lead holds the feature, not qa",
		},
		{
			"the change leaves an invalid flow",
			func(feature *state.Feature) error {
				feature.Flow.Holder = "boss"
				return nil
			},
			`the change leaves feature demo-1 invalid: flow: unknown holder "boss"`,
		},
		{
			"the change renames the feature",
			func(feature *state.Feature) error {
				feature.Slug = "demo-2"
				return nil
			},
			`the change leaves feature demo-1 invalid: slug "demo-2": want "demo-1"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := created(t)
			before := read(t, featureFile(store))

			got, err := store.Update(slug, test.change)
			wantError(t, err, test.want)

			if got != (state.Feature{}) {
				t.Errorf("Update = %+v, want no feature", got)
			}

			if after := read(t, featureFile(store)); after != before {
				t.Errorf("the feature file holds\n%s\nwant it unchanged:\n%s", after, before)
			}
		})
	}

	_, err := created(t).Update(slug, func(*state.Feature) error { return refused })
	if !errors.Is(err, refused) {
		t.Errorf("Update error = %v, want the change's error", err)
	}
}

func TestUpdateKeepsTheFieldsItDoesntKnow(t *testing.T) {
	store := holding(t, `{
  "format": 1,
  "slug": "demo-1",
  "branch": "baton/demo-1",
  "flow": {"holder": "lead", "status": "running", "round": 0, "cap": 3, "extra": 0},
  "sessions": {"lead": "1"}
}`)

	if _, err := store.Update(slug, hand(flow.Handover{From: flow.RoleLead})); err != nil {
		t.Fatal(err)
	}

	got := read(t, featureFile(store))
	for _, want := range []string{`"holder": "engineer"`, `"branch": "baton/demo-1"`, `"lead": "1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the feature file holds\n%s\nwant %s in it", got, want)
		}
	}
}

func TestUpdateLosesNoChange(t *testing.T) {
	const changes = 20

	store := created(t)
	count := func(feature *state.Feature) error {
		feature.Flow.Cap++
		return nil
	}

	var workers sync.WaitGroup
	for range changes {
		workers.Go(func() {
			if _, err := store.Update(slug, count); err != nil {
				t.Error(err)
			}
		})
		workers.Go(func() {
			if _, err := store.Read(slug); err != nil {
				t.Error(err)
			}
		})
	}

	workers.Wait()

	got, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	if want := started(t).Flow.Cap + changes; got.Flow.Cap != want {
		t.Errorf("after %d changes, the cap is %d, want %d", changes, got.Flow.Cap, want)
	}
}

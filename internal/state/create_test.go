package state_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/codedogapp/baton/internal/state"
)

func TestCreateWritesTheFeatureFile(t *testing.T) {
	want := `{
  "format": 1,
  "slug": "demo-1",
  "flow": {
    "holder": "lead",
    "status": "running",
    "round": 0,
    "cap": 3,
    "extra": 0
  }
}
`
	if got := read(t, featureFile(created(t))); got != want {
		t.Errorf("the feature file holds\n%s\nwant\n%s", got, want)
	}
}

func TestCreateKeepsTheStateToTheUser(t *testing.T) {
	store := created(t)
	folder := filepath.Dir(featureFile(store))

	modes := map[string]fs.FileMode{
		store.Dir:                      0o700,
		filepath.Dir(folder):           0o700,
		folder:                         0o700,
		featureFile(store):             0o600,
		filepath.Join(folder, "lock"):  0o600,
		filepath.Join(folder, "notes"): 0o700,
		filepath.Join(folder, "notes", "01-orchestrator-to-lead.md"): 0o600,
		filepath.Join(folder, "events.ndjson"):                       0o600,
	}

	for path, want := range modes {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}

		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s has mode %v, want %v", path, got, want)
		}
	}
}

func TestReadReturnsTheCreatedFeature(t *testing.T) {
	got, err := created(t).Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	if want := started(t); got != want {
		t.Errorf("Read = %+v, want %+v", got, want)
	}
}

func TestCreateRefusesATakenSlug(t *testing.T) {
	store := created(t)
	before := read(t, featureFile(store))

	wantError(t, store.Create(started(t), briefed()), "feature demo-1 exists")

	if after := read(t, featureFile(store)); after != before || len(events(t, store)) != 1 {
		t.Errorf("the feature file holds\n%s\nwant it unchanged:\n%s", after, before)
	}

	wantNotes(t, store, "01-orchestrator-to-lead.md")
}

func TestCreateHasOneWinner(t *testing.T) {
	const tries = 20

	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}
	feature := started(t)
	won := make(chan bool, tries)

	var workers sync.WaitGroup
	for range tries {
		workers.Go(func() {
			err := store.Create(feature, briefed())
			if err != nil && !strings.Contains(err.Error(), "feature demo-1 exists") {
				t.Error(err)
			}

			won <- err == nil
		})
	}

	workers.Wait()
	close(won)

	wins := 0

	for one := range won {
		if one {
			wins++
		}
	}

	if wins != 1 {
		t.Errorf("%d of %d Creates won, want 1", wins, tries)
	}

	if logged := events(t, store); len(logged) != 1 {
		t.Errorf("the log holds %+v, want one start", logged)
	}

	wantNotes(t, store, "01-orchestrator-to-lead.md")
}

func TestCreateRefusesAnInvalidFeature(t *testing.T) {
	store := state.Store{Dir: t.TempDir()}

	wantError(t, store.Create(state.Feature{Slug: slug}, briefed()),
		`new feature demo-1: flow: unknown holder ""`)
	wantNothingIn(t, store.Dir)
}

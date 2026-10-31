package state_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

func TestListReturnsTheFeaturesBySlug(t *testing.T) {
	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}
	escalated := flow.State{
		Holder: flow.RoleOrchestrator, Status: flow.StatusEscalated, Round: 4, Cap: 3, Extra: 1,
	}
	want := []state.Feature{
		{Slug: "demo-1", Flow: started(t).Flow},
		{Slug: "demo-2", Flow: escalated},
	}

	for _, feature := range slices.Backward(want) {
		if err := store.Create(feature, briefed()); err != nil {
			t.Fatal(err)
		}
	}

	// What isn't a feature's folder: a file, a folder whose name isn't a slug, and the folder of a
	// feature that Create hasn't written yet.
	features := filepath.Join(store.Dir, "features")
	writeFile(t, filepath.Join(features, "demo-4"), "Not a folder.\n")
	writeFile(t, filepath.Join(features, "demo_1", "feature.json"), read(t, featureFile(store)))

	if err := os.Mkdir(filepath.Join(features, "demo-3"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := store.List()
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("List = %+v, %v; want %+v", got, err, want)
	}
}

func TestListReturnsNothingWithoutFeatures(t *testing.T) {
	empty := t.TempDir()
	if err := os.Mkdir(filepath.Join(empty, "features"), 0o700); err != nil {
		t.Fatal(err)
	}

	stores := map[string]state.Store{
		"no baton folder":       {Dir: filepath.Join(t.TempDir(), "baton")},
		"no features folder":    {Dir: t.TempDir()},
		"empty features folder": {Dir: empty},
	}

	for name, store := range stores {
		if got, err := store.List(); err != nil || got != nil {
			t.Errorf("%s: List = %+v, %v; want nothing", name, got, err)
		}
	}
}

func TestListReportsABrokenFeature(t *testing.T) {
	store := holding(t, `{"format": 2}`)

	_, err := store.List()
	wantError(t, err, "feature file "+featureFile(store)+": format 2 is newer")
}

func TestListReportsAFeaturesFolderItCantRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "features"), "")

	_, err := state.Store{Dir: dir}.List()
	wantError(t, err, "list the features: ")
}

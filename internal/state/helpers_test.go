package state_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// slug is the feature that the tests store.
const slug = "demo-1"

// started returns a new feature, which the lead holds.
func started(t *testing.T) state.Feature {
	t.Helper()

	start, err := flow.Start(3)
	if err != nil {
		t.Fatal(err)
	}

	return state.Feature{Slug: slug, Flow: start}
}

// featureFile returns the path of the feature's file in the store.
func featureFile(store state.Store) string {
	return filepath.Join(store.Dir, "features", slug, "feature.json")
}

// holding returns a store in a new folder, whose feature's file holds the content.
func holding(t *testing.T, content string) state.Store {
	t.Helper()

	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}
	writeFile(t, featureFile(store), content)

	return store
}

// writeFile writes the content to a new file at the path, after making its folders.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// wantError fails the test unless the error's message has want in it.
func wantError(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want one with %q", err, want)
	}
}

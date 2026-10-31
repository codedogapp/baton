package state_test

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The feature that the tests store, and the orchestrator's brief that starts it.
const (
	slug  = "demo-1"
	brief = "Build the demo.\n"
)

// started returns a new feature, which the lead holds.
func started(t *testing.T) state.Feature {
	t.Helper()

	start, err := flow.Start(3)
	if err != nil {
		t.Fatal(err)
	}

	return state.Feature{Slug: slug, Flow: start}
}

// briefed returns the entry that starts the feature: the orchestrator's brief to the lead.
func briefed() state.Entry {
	return state.Entry{
		Event: state.Event{Kind: state.EventStart, Role: flow.RoleOrchestrator},
		Note:  []byte(brief),
	}
}

// briefing returns the holder's work after the orchestrator's brief, the feature's first note.
func briefing(holder flow.Role) state.Work {
	return state.Work{
		Note: "01-orchestrator-to-" + string(holder) + ".md",
		From: flow.RoleOrchestrator,
	}
}

// stored returns the new feature as Create stores it: with the brief as the lead's work.
func stored(t *testing.T) state.Feature {
	t.Helper()

	feature := started(t)
	feature.Work = briefing(flow.RoleLead)

	return feature
}

// logged returns an entry with an event alone: the lead's handover, without a note.
func logged() state.Entry {
	return state.Entry{Event: state.Event{Kind: state.EventHandover, Role: flow.RoleLead}}
}

// noted returns the entry of the role's handover, with its verdict and its note.
func noted(role flow.Role, verdict flow.Verdict) state.Entry {
	return state.Entry{
		Event: state.Event{Kind: state.EventHandover, Role: role, Verdict: verdict},
		Note:  []byte("From the " + string(role) + ".\n"),
	}
}

// created returns a store in a new folder, with the new feature in it.
func created(t *testing.T) state.Store {
	t.Helper()

	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}
	if err := store.Create(started(t), briefed()); err != nil {
		t.Fatal(err)
	}

	return store
}

// featureFile returns the path of the feature's file in the store.
func featureFile(store state.Store) string {
	return filepath.Join(store.Dir, "features", slug, "feature.json")
}

// eventsFile returns the path of the feature's event log in the store.
func eventsFile(store state.Store) string {
	return filepath.Join(store.Dir, "features", slug, "events.ndjson")
}

// notesFolder returns the path of the feature's notes folder in the store.
func notesFolder(store state.Store) string {
	return filepath.Join(store.Dir, "features", slug, "notes")
}

// events returns the events in the feature's log, in order.
func events(t *testing.T, store state.Store) []state.Event {
	t.Helper()

	var logged []state.Event

	for line := range strings.Lines(read(t, eventsFile(store))) {
		var event state.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event %q: %v", line, err)
		}

		logged = append(logged, event)
	}

	return logged
}

// wantNotes fails the test unless the feature's notes folder holds the notes, and nothing else.
func wantNotes(t *testing.T, store state.Store, want ...string) {
	t.Helper()

	entries, err := os.ReadDir(notesFolder(store))
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}

	if !slices.Equal(got, want) {
		t.Errorf("the notes folder holds %q, want %q", got, want)
	}
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

// read returns the content of the file at the path.
func read(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// wantError fails the test unless the error's message has want in it.
func wantError(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want one with %q", err, want)
	}
}

// wantNothingIn fails the test unless the folder is empty.
func wantNothingIn(t *testing.T, folder string) {
	t.Helper()

	if entries, err := os.ReadDir(folder); err != nil || len(entries) > 0 {
		t.Errorf("%s holds %v (%v), want nothing", folder, entries, err)
	}
}

package state_test

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// handedOver returns a store with the new feature, which the lead has handed over to the
// engineer, with a note: two notes and two events.
func handedOver(t *testing.T) state.Store {
	t.Helper()

	store := created(t)

	_, err := store.Update(slug, noted(flow.RoleLead, ""), hand(flow.Handover{From: flow.RoleLead}))
	if err != nil {
		t.Fatal(err)
	}

	return store
}

// wantFolder fails the test unless the feature's folder holds the files, and nothing else.
func wantFolder(t *testing.T, store state.Store, want ...string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Dir(featureFile(store)))
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}

	if !slices.Equal(got, want) {
		t.Errorf("the feature's folder holds %q, want %q", got, want)
	}
}

func TestRemoveRemovesAllButTheLock(t *testing.T) {
	store := handedOver(t)

	if err := store.Remove(slug); err != nil {
		t.Fatal(err)
	}

	_, readErr := store.Read(slug)
	_, updateErr := store.Update(slug, logged(), keep)

	for call, err := range map[string]error{"Read": readErr, "Update": updateErr} {
		if err == nil || err.Error() != "no feature demo-1" {
			t.Errorf("%s error = %v, want no feature demo-1", call, err)
		}
	}

	if features, err := store.List(); err != nil || len(features) > 0 {
		t.Errorf("List = %+v, %v; want no features", features, err)
	}

	wantFolder(t, store, "lock")
}

func TestRemoveLeavesTheSlugToANewFeature(t *testing.T) {
	store := handedOver(t)

	if err := store.Remove(slug); err != nil {
		t.Fatal(err)
	}

	if err := store.Create(started(t), briefed()); err != nil {
		t.Fatal(err)
	}

	if got, err := store.Read(slug); err != nil || got != stored(t) {
		t.Errorf("Read = %+v, %v; want %+v", got, err, stored(t))
	}

	wantNotes(t, store, "01-orchestrator-to-lead.md")

	if logged := events(t, store); len(logged) != 1 || logged[0].Kind != state.EventStart {
		t.Errorf("the log holds %+v, want the new feature's start alone", logged)
	}
}

func TestRemoveWaitsForTheFeaturesLock(t *testing.T) {
	store := created(t)

	lock, err := os.OpenFile(filepath.Join(filepath.Dir(featureFile(store)), "lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	removed := make(chan error, 1)

	go func() { removed <- store.Remove(slug) }()

	select {
	case err := <-removed:
		t.Fatalf("Remove = %v while another process held the lock; want it to wait", err)
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := store.Read(slug); err != nil {
		t.Errorf("Read = %v while Remove waits for the lock; want the feature", err)
	}

	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}

	if err := <-removed; err != nil {
		t.Fatal(err)
	}
}

func TestRemoveReportsWhatItCantRemove(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}

	t.Run("the feature's file", func(t *testing.T) {
		store := handedOver(t)
		before := read(t, featureFile(store))
		chmod(0o500)(t, filepath.Dir(featureFile(store)))

		wantError(t, store.Remove(slug), "remove feature demo-1: ")

		if after := read(t, featureFile(store)); after != before || len(events(t, store)) != 2 {
			t.Errorf("Remove changed the feature, which it couldn't remove")
		}

		wantNotes(t, store, "01-orchestrator-to-lead.md", "02-lead-to-engineer.md")
	})

	t.Run("the notes", func(t *testing.T) {
		store := handedOver(t)
		chmod(0o500)(t, notesFolder(store))

		wantError(t, store.Remove(slug), "remove feature demo-1: ")

		if _, err := store.Read(slug); err == nil || err.Error() != "no feature demo-1" {
			t.Errorf("Read error = %v, want no feature demo-1: the feature's file goes first", err)
		}

		wantFolder(t, store, "lock", "notes")
	})

	t.Run("the event log", func(t *testing.T) {
		store := handedOver(t)
		full := filepath.Join(eventsFile(store), "full")

		// A folder in the event log's place, with a file that baton can't remove.
		if err := os.Remove(eventsFile(store)); err != nil {
			t.Fatal(err)
		}

		writeFile(t, filepath.Join(full, "event"), "")
		chmod(0o500)(t, full)

		wantError(t, store.Remove(slug), "remove feature demo-1: ")
		wantFolder(t, store, "events.ndjson", "lock")
	})
}

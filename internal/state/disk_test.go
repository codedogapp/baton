package state_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

func TestUpdateWritesOverALeftoverTemporaryFile(t *testing.T) {
	store := created(t)
	temp := featureFile(store) + ".tmp"

	if err := os.WriteFile(temp, []byte(`{"format": 1, "sl`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Update(slug, logged(), keep); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(temp); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the temporary file is still there (%v), want it renamed", err)
	}
}

func TestUpdateReportsAFileItCantWrite(t *testing.T) {
	store := created(t)
	before := read(t, featureFile(store))

	// A folder in the temporary file's place, which baton can't remove.
	if err := os.MkdirAll(filepath.Join(featureFile(store)+".tmp", "full"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := store.Update(slug, logged(), keep)
	wantError(t, err, "write feature demo-1: ")

	if after := read(t, featureFile(store)); after != before {
		t.Errorf("the feature file holds\n%s\nwant it unchanged:\n%s", after, before)
	}
}

func TestCreateAndUpdateReportALockTheyCantTake(t *testing.T) {
	store := created(t)
	lock := filepath.Join(filepath.Dir(featureFile(store)), "lock")

	// A folder in the lock's place.
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(lock, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := store.Update(slug, logged(), keep)
	wantError(t, err, "lock feature demo-1: ")
	wantError(t, store.Create(started(t), briefed()), "lock feature demo-1: ")
}

func TestUpdateWritesNothingWhenItCantKeepTheNote(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}

	breaks := map[string]func(t *testing.T, notes string){
		"a file in the notes folder's place": func(t *testing.T, notes string) {
			t.Helper()

			if err := os.RemoveAll(notes); err != nil {
				t.Fatal(err)
			}

			writeFile(t, notes, "")
		},
		"a notes folder baton can't read":  chmod(0o300),
		"a notes folder baton can't write": chmod(0o500),
	}

	for name, broken := range breaks {
		t.Run(name, func(t *testing.T) {
			store := created(t)
			before := read(t, featureFile(store))
			broken(t, notesFolder(store))

			_, err := store.Update(
				slug,
				noted(flow.RoleLead, ""),
				hand(flow.Handover{From: flow.RoleLead}),
			)
			wantError(t, err, "keep the note of feature demo-1: ")

			if after := read(t, featureFile(store)); after != before || len(events(t, store)) != 1 {
				t.Errorf("Update wrote a change without its note")
			}
		})
	}
}

// chmod returns a change of the folder's mode, which the test undoes when it ends.
func chmod(mode os.FileMode) func(t *testing.T, folder string) {
	return func(t *testing.T, folder string) {
		t.Helper()

		info, err := os.Stat(folder)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chmod(folder, mode); err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = os.Chmod(folder, info.Mode()) })
	}
}

func TestUpdateReportsAnEventItCantLog(t *testing.T) {
	store := created(t)
	if err := os.Remove(eventsFile(store)); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(eventsFile(store), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := store.Update(slug, noted(flow.RoleLead, ""), hand(flow.Handover{From: flow.RoleLead}))
	wantError(t, err, "feature demo-1 changed, but its event log didn't: ")

	if got, err := store.Read(slug); err != nil || got.Flow.Holder != flow.RoleEngineer {
		t.Errorf("Read = %+v, %v; want the change made", got, err)
	}
}

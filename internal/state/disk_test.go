package state_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateWritesOverALeftoverTemporaryFile(t *testing.T) {
	store := created(t)
	temp := featureFile(store) + ".tmp"

	if err := os.WriteFile(temp, []byte(`{"format": 1, "sl`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Update(slug, keep); err != nil {
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

	_, err := store.Update(slug, keep)
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

	_, err := store.Update(slug, keep)
	wantError(t, err, "lock feature demo-1: ")
	wantError(t, store.Create(started(t)), "lock feature demo-1: ")
}

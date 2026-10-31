package state_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/state"
)

func TestTheStoreRefusesABadSlug(t *testing.T) {
	store := state.Store{Dir: t.TempDir()}
	bads := []string{
		"", "Demo-1", "demo_1", "-demo", "demo-", "de--mo", "..", "../demo", "a/b", "demo.1",
		"démo", "demo 1",
	}

	for _, bad := range bads {
		feature := started(t)
		feature.Slug = bad
		_, readErr := store.Read(bad)
		_, updateErr := store.Update(bad, keep)

		errs := map[string]error{
			"Create": store.Create(feature),
			"Read":   readErr,
			"Update": updateErr,
		}
		for call, err := range errs {
			want := fmt.Sprintf("slug %q: want words of lowercase letters and digits", bad)
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Errorf("%s(%q) error = %v, want one about the slug", call, bad, err)
			}
		}
	}

	wantNothingIn(t, store.Dir)
}

func TestReadAndUpdateReportAMissingFeature(t *testing.T) {
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "features", "demo-2", "feature.json"), "{}")

	bare := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bare, "features", slug), 0o700); err != nil {
		t.Fatal(err)
	}

	stores := map[string]state.Store{
		"no baton folder":           {Dir: filepath.Join(t.TempDir(), "baton")},
		"another feature only":      {Dir: other},
		"the feature's folder only": {Dir: bare},
	}

	for name, store := range stores {
		_, readErr := store.Read(slug)
		_, updateErr := store.Update(slug, keep)

		for call, err := range map[string]error{"Read": readErr, "Update": updateErr} {
			if err == nil || err.Error() != "no feature demo-1" {
				t.Errorf("%s: %s error = %v, want no feature demo-1", name, call, err)
			}
		}
	}
}

func TestTheStoreReportsAFolderItCantUse(t *testing.T) {
	file := filepath.Join(t.TempDir(), "baton")
	writeFile(t, file, "")

	store := state.Store{Dir: file}
	_, readErr := store.Read(slug)
	_, updateErr := store.Update(slug, keep)

	wantError(t, store.Create(started(t)), "make the folder of feature demo-1: ")
	wantError(t, readErr, "open baton's folder: ")
	wantError(t, updateErr, "open baton's folder: ")
}

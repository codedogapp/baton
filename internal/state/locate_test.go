package state_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/state"
)

func TestReadRefusesABadSlug(t *testing.T) {
	store := state.Store{Dir: t.TempDir()}
	bads := []string{
		"", "Demo-1", "demo_1", "-demo", "demo-", "de--mo", "..", "../demo", "a/b", "demo.1",
		"démo", "demo 1",
	}

	for _, bad := range bads {
		_, err := store.Read(bad)

		want := fmt.Sprintf("slug %q: want words of lowercase letters and digits", bad)
		if err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("Read(%q) error = %v, want one about the slug", bad, err)
		}
	}
}

func TestReadReportsAMissingFeature(t *testing.T) {
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
		if _, err := store.Read(slug); err == nil || err.Error() != "no feature demo-1" {
			t.Errorf("%s: error = %v, want no feature demo-1", name, err)
		}
	}
}

func TestReadReportsAFolderItCantUse(t *testing.T) {
	file := filepath.Join(t.TempDir(), "baton")
	writeFile(t, file, "")

	_, err := state.Store{Dir: file}.Read(slug)
	wantError(t, err, "open baton's folder: ")
}

package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/state"
)

func TestReadReturnsTheFeatureInItsFile(t *testing.T) {
	store := holding(t, `{
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
`)

	got, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	if want := started(t); got != want {
		t.Errorf("Read = %+v, want %+v", got, want)
	}
}

func TestReadIgnoresTheFieldsItDoesntKnow(t *testing.T) {
	store := holding(t, `{
  "format": 1,
  "slug": "demo-1",
  "branch": "baton/demo-1",
  "flow": {"holder": "lead", "status": "running", "round": 0, "cap": 3, "extra": 0, "paused": true},
  "sessions": {"lead": "1"}
}`)

	if got, err := store.Read(slug); err != nil || got != started(t) {
		t.Errorf("Read = %+v, %v; want %+v", got, err, started(t))
	}
}

func TestReadRefusesAnInvalidFile(t *testing.T) {
	const lead = `"flow": {"holder": "lead", "status": "running", "round": 0, "cap": 3, "extra": 0}`

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"no JSON", "lead", "invalid character 'l'"},
		{"no object", "[]", "unmarshal JSON array into Go struct"},
		{"two values", `{"format": 1} {}`, "after top-level value"},
		{"a field twice", `{"format": 1, "format": 1}`, `duplicate object member name "format"`},
		{"no format", `{"slug": "demo-1", ` + lead + `}`, "format 0: want 1"},
		{
			"a newer format, with fields that this baton can't read",
			`{"format": 2, "flow": "elsewhere"}`,
			"format 2 is newer than this baton's, 1: run the newer baton",
		},
		{
			"a field of the wrong type",
			`{"format": 1, "slug": "demo-1", "flow": {"round": "one"}}`,
			`unmarshal JSON string into Go int within "/flow/round"`,
		},
		{
			"another feature's slug",
			`{"format": 1, "slug": "demo-2", ` + lead + `}`,
			`slug "demo-2": want "demo-1", the feature's folder`,
		},
		{
			"an invalid flow",
			`{"format": 1, "slug": "demo-1", "flow": {"holder": "boss"}}`,
			`flow: unknown holder "boss"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := holding(t, test.content)

			_, err := store.Read(slug)
			wantError(t, err, "feature file "+featureFile(store)+": ")
			wantError(t, err, test.want)
		})
	}
}

func TestReadReportsAFileItCantRead(t *testing.T) {
	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}

	// A folder in the file's place.
	if err := os.MkdirAll(featureFile(store), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := store.Read(slug)
	wantError(t, err, "read feature demo-1: ")
}

package state_test

import (
	"encoding/json/v2"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"uuid"

	"github.com/codedogapp/baton/internal/state"
)

// orchestratorFile returns the path of the orchestrator's file in the store.
func orchestratorFile(store state.Store) string {
	return filepath.Join(store.Dir, "orchestrator.json")
}

// wantSession fails the test unless the store keeps the session as the orchestrator's.
func wantSession(t *testing.T, store state.Store, want uuid.UUID) {
	t.Helper()

	if got, err := store.OrchestratorSession(); err != nil || got != want {
		t.Errorf("OrchestratorSession = %v, %v; want %v", got, err, want)
	}
}

func TestTheOrchestratorsSessionIsTheLastOneKept(t *testing.T) {
	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}
	first, last := uuid.New(), uuid.New()

	wantSession(t, store, uuid.Nil())

	for _, session := range []uuid.UUID{first, last} {
		if err := store.KeepOrchestratorSession(session); err != nil {
			t.Fatal(err)
		}

		wantSession(t, store, session)
	}

	want := "{\n  \"format\": 1,\n  \"session\": \"" + last.String() + "\"\n}\n"
	if got := read(t, orchestratorFile(store)); got != want {
		t.Errorf("the orchestrator's file holds\n%s\nwant\n%s", got, want)
	}

	modes := map[string]fs.FileMode{
		store.Dir:               0o700,
		orchestratorFile(store): 0o600,
		filepath.Join(store.Dir, "orchestrator.lock"): 0o600,
	}

	for path, want := range modes {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v, %v; want mode %v", path, info, err, want)
		}
	}
}

func TestTheOrchestratorsSessionIsNoneWithoutItsFile(t *testing.T) {
	wantSession(t, created(t), uuid.Nil())
	wantSession(t, holdingOrchestrator(t, `{"format": 1}`), uuid.Nil())
}

func TestKeepOrchestratorSessionKeepsTheFieldsItDoesntKnow(t *testing.T) {
	store := holdingOrchestrator(t, `{"format": 1, "session": "`+uuid.New().String()+
		`", "panes": {"lead": "%1"}}`)
	session := uuid.New()

	if err := store.KeepOrchestratorSession(session); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(read(t, orchestratorFile(store))), &got); err != nil {
		t.Fatal(err)
	}

	panes, _ := got["panes"].(map[string]any)
	if got["session"] != session.String() || panes["lead"] != "%1" {
		t.Errorf("the orchestrator's file holds %v, want the new session and the panes", got)
	}
}

// holdingOrchestrator returns a store in a new folder, whose orchestrator's file holds the
// content.
func holdingOrchestrator(t *testing.T, content string) state.Store {
	t.Helper()

	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}
	writeFile(t, orchestratorFile(store), content)

	return store
}

func TestTheOrchestratorsSessionRefusesAnInvalidFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"no JSON", "lead", "invalid character 'l'"},
		{"no format", `{"session": "` + uuid.New().String() + `"}`, "format 0: want 1"},
		{
			"a newer format",
			`{"format": 2, "session": 2}`,
			"format 2 is newer than this baton's, 1: run the newer baton",
		},
		{"a session's ID that isn't one", `{"format": 1, "session": "1"}`, `within "/session"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := holdingOrchestrator(t, test.content)
			_, readErr := store.OrchestratorSession()

			for _, err := range []error{readErr, store.KeepOrchestratorSession(uuid.New())} {
				wantError(t, err, "orchestrator file "+orchestratorFile(store)+": ")
				wantError(t, err, test.want)
			}

			if got := read(t, orchestratorFile(store)); got != test.content {
				t.Errorf("the orchestrator's file holds %s, want it unchanged", got)
			}
		})
	}
}

func TestKeepOrchestratorSessionRefusesTheNilUUID(t *testing.T) {
	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}

	wantError(t, store.KeepOrchestratorSession(uuid.Nil()), "want its ID, not the nil UUID")

	if _, err := os.Stat(store.Dir); err == nil {
		t.Error("KeepOrchestratorSession made baton's folder for the nil UUID")
	}
}

func TestKeepOrchestratorSessionHasOneWriterAtATime(t *testing.T) {
	const tries = 20

	store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}

	var workers sync.WaitGroup
	for range tries {
		workers.Go(func() {
			if err := store.KeepOrchestratorSession(uuid.New()); err != nil {
				t.Error(err)
			}
		})
	}

	workers.Wait()

	if got, err := store.OrchestratorSession(); err != nil || got == uuid.Nil() {
		t.Errorf("OrchestratorSession = %v, %v; want one of the sessions", got, err)
	}
}

func TestTheOrchestratorsSessionReportsWhatItCantUse(t *testing.T) {
	// A folder in a file's place.
	inPlace := func(t *testing.T, name string) state.Store {
		t.Helper()

		store := state.Store{Dir: filepath.Join(t.TempDir(), "baton")}
		if err := os.MkdirAll(filepath.Join(store.Dir, name, "full"), 0o700); err != nil {
			t.Fatal(err)
		}

		return store
	}

	session := uuid.New()
	file := filepath.Join(t.TempDir(), "baton")
	writeFile(t, file, "")

	_, readErr := state.Store{Dir: file}.OrchestratorSession()
	wantError(t, readErr, "open baton's folder: ")
	wantError(t, state.Store{Dir: file}.KeepOrchestratorSession(session), "make baton's folder: ")

	_, readErr = inPlace(t, "orchestrator.json").OrchestratorSession()
	wantError(t, readErr, "read the orchestrator's file: ")

	keepErr := inPlace(t, "orchestrator.json").KeepOrchestratorSession(session)
	wantError(t, keepErr, "read the orchestrator's file: ")

	keepErr = inPlace(t, "orchestrator.lock").KeepOrchestratorSession(session)
	wantError(t, keepErr, "lock the orchestrator's file: ")

	keepErr = inPlace(t, "orchestrator.json.tmp").KeepOrchestratorSession(session)
	wantError(t, keepErr, "write the orchestrator's file: ")
}

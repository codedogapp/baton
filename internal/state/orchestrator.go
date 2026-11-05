package state

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"uuid"
)

// orchestratorFormat is the layout of the orchestrator's file that this baton reads and writes. As
// the feature file's format does, it changes only when an older baton would misread the new
// layout (decision 24).
const orchestratorFormat = 1

// The orchestrator's file in baton's folder, and its lock, a file that baton never replaces.
const (
	orchestratorFile = "orchestrator.json"
	orchestratorLock = "orchestrator.lock"
)

// orchestrator is the orchestrator's file's content: the file's format, the ID of the copilot
// session that the orchestrator's pane resumes, and the fields that this baton doesn't know,
// which a newer baton wrote.
type orchestrator struct {
	Format  int            `json:"format"`
	Session uuid.UUID      `json:"session"`
	Unknown jsontext.Value `json:",embed"`
}

// OrchestratorSession returns the ID of the copilot session that the orchestrator's pane resumes,
// which KeepOrchestratorSession kept, or the nil UUID if it kept none (decision 68). Like Read,
// it takes no lock.
func (s Store) OrchestratorSession() (uuid.UUID, error) {
	root, err := os.OpenRoot(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return uuid.Nil(), nil
	}

	if err != nil {
		return uuid.Nil(), fmt.Errorf("open baton's folder: %w", err)
	}

	defer func() { _ = root.Close() }()

	rec, err := loadOrchestrator(root)

	return rec.Session, err
}

// KeepOrchestratorSession keeps the session's ID as the one that the orchestrator's pane resumes,
// under the orchestrator's lock, in a new file that it renames over the old one, as Update does.
// The fields of the file that this baton doesn't know stay as they are.
func (s Store) KeepOrchestratorSession(session uuid.UUID) error {
	if session == uuid.Nil() {
		return errors.New("keep the orchestrator's session: want its ID, not the nil UUID")
	}

	if err := os.MkdirAll(s.Dir, privateFolder); err != nil {
		return fmt.Errorf("make baton's folder: %w", err)
	}

	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return fmt.Errorf("open baton's folder: %w", err)
	}

	defer func() { _ = root.Close() }()

	unlock, err := take(root, orchestratorLock)
	if err != nil {
		return fmt.Errorf("lock the orchestrator's file: %w", err)
	}

	defer unlock()

	rec, err := loadOrchestrator(root)
	if err != nil {
		return err
	}

	rec.Format, rec.Session = orchestratorFormat, session

	if err := replace(root, orchestratorFile, rec); err != nil {
		return fmt.Errorf("write the orchestrator's file: %w", err)
	}

	return nil
}

// loadOrchestrator reads the orchestrator's file and checks its format. A missing file keeps no
// session.
func loadOrchestrator(root *os.Root) (orchestrator, error) {
	data, err := root.ReadFile(orchestratorFile)
	if errors.Is(err, fs.ErrNotExist) {
		return orchestrator{}, nil
	}

	if err != nil {
		return orchestrator{}, fmt.Errorf("read the orchestrator's file: %w", err)
	}

	rec, err := decode[orchestrator](data, orchestratorFormat)
	if err != nil {
		return orchestrator{}, fmt.Errorf(
			"orchestrator file %s: %w", filepath.Join(root.Name(), orchestratorFile), err)
	}

	return rec, nil
}

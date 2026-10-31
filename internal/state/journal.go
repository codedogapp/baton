package state

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codedogapp/baton/internal/flow"
)

// The feature's journal in its folder.
const (
	eventsFile   = "events.ndjson"
	notesFolder  = "notes"
	eventsAppend = os.O_RDWR | os.O_APPEND | os.O_CREATE
)

// The events. Each command that changes a feature logs its own.
const (
	EventStart    = "start"    // The orchestrator started the feature, with its brief to the lead.
	EventHandover = "handover" // The holder handed the feature over, with a note.
	EventPickup   = "pickup"   // The holder's baton wait picked up its work.
)

// Entry is what a change adds to the feature's journal: its event, and the note that came with
// it, if any. Every change logs an event.
type Entry struct {
	Event Event
	// Note is the note's text, if any. The store keeps it as notes/NN-<from>-to-<to>.md: from the event's
	// role to the holder after the change, numbered after the folder's last note.
	Note []byte
}

// Event is a line of the feature's event log, events.ndjson. The store sets its time, to the
// millisecond, its note and its flow.
type Event struct {
	Time    time.Time    `json:"time"`
	Kind    string       `json:"event"`
	Role    flow.Role    `json:"role"` // The role that acted.
	Verdict flow.Verdict `json:"verdict,omitzero"`
	Note    string       `json:"note,omitzero"` // The name of the note in the notes folder.
	Flow    flow.State   `json:"flow"`          // Where the feature stands after the event.
}

// check returns an error if the event doesn't say what happened and who did it.
func (e Event) check() error {
	if e.Kind == "" || e.Role == "" {
		return fmt.Errorf("event %q by %q: want its kind and its role", e.Kind, e.Role)
	}

	return nil
}

// commit writes the record and its entry, under the feature's lock: the note first, as a new file
// that nothing names yet; then the record, which makes the change; then the event, so the log
// never shows a change that didn't happen. The note becomes the holder's work.
func commit(root *os.Root, rec *record, entry Entry) error {
	event := entry.Event
	event.Flow = rec.Flow

	if len(entry.Note) > 0 {
		name, err := keepNote(root, *rec, entry)
		if err != nil {
			return fmt.Errorf("keep the note of feature %s: %w", rec.Slug, err)
		}

		event.Note = name
		rec.Work = Work{Note: name, From: event.Role, Verdict: event.Verdict}
	}

	if err := save(root, *rec); err != nil {
		return err
	}

	event.Time = time.Now().Truncate(time.Millisecond)
	if err := logEvent(root, rec.Slug, event); err != nil {
		return fmt.Errorf("feature %s changed, but its event log didn't: %w", rec.Slug, err)
	}

	return nil
}

// keepNote writes the entry's note to a new file in the feature's notes folder, named from the
// event's role to the holder after the change, and returns its name.
func keepNote(root *os.Root, rec record, entry Entry) (string, error) {
	notes := filepath.Join(folder(rec.Slug), notesFolder)
	if err := root.MkdirAll(notes, privateFolder); err != nil {
		return "", err
	}

	last, err := lastNote(root, notes)
	if err != nil {
		return "", err
	}

	name := fmt.Sprintf("%02d-%s-to-%s.md", last+1, entry.Event.Role, rec.Flow.Holder)

	return name, write(root, filepath.Join(notes, name), entry.Note)
}

// lastNote returns the highest number of the notes in the folder, or 0 if it has none.
func lastNote(root *os.Root, notes string) (int, error) {
	entries, err := fs.ReadDir(root.FS(), filepath.ToSlash(notes))
	if err != nil {
		return 0, err
	}

	last := 0

	for _, entry := range entries {
		number, _, _ := strings.Cut(entry.Name(), "-")
		if n, notNumber := strconv.Atoi(number); notNumber == nil {
			last = max(last, n)
		}
	}

	return last, nil
}

// logEvent appends the event to the feature's event log, as one line, and syncs it to disk. The
// caller holds the lock, so no one makes the file at the same time (fact 19).
func logEvent(root *os.Root, slug string, event Event) error {
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}

	file, err := root.OpenFile(filepath.Join(folder(slug), eventsFile), eventsAppend, privateFile)
	if err != nil {
		return err
	}

	if torn(file) {
		line = append([]byte{'\n'}, line...)
	}

	_, err = file.Write(append(line, '\n'))
	if err == nil {
		err = file.Sync()
	}

	return errors.Join(err, file.Close())
}

// torn reports whether the file's last line lacks its newline, as after a crash in a write, so
// that the next event starts a line of its own.
func torn(file *os.File) bool {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return false
	}

	last := make([]byte, 1)
	_, err = file.ReadAt(last, info.Size()-1)

	return err == nil && last[0] != '\n'
}

package state_test

import (
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

func TestCreateStartsTheJournal(t *testing.T) {
	before := time.Now().Truncate(time.Millisecond)
	store := created(t)
	after := time.Now()

	line := read(t, eventsFile(store))
	stamp := regexp.MustCompile(`"time":"([-0-9]+T[:0-9]+(\.[0-9]{1,3})?(Z|[-+][:0-9]+))"`)

	match := stamp.FindStringSubmatch(line)
	if match == nil {
		t.Fatalf("the event log holds %q, want a time to the millisecond in it", line)
	}

	when, err := time.Parse(time.RFC3339Nano, match[1])
	if err != nil || when.Before(before) || when.After(after) {
		t.Errorf("the event's time is %s (%v), want one from %s to %s", when, err, before, after)
	}

	want := `{"time":"…","event":"start","role":"orchestrator","note":"01-orchestrator-to-lead.md",` +
		`"flow":{"holder":"lead","status":"running","round":0,"cap":3,"extra":0}}` + "\n"
	if got := stamp.ReplaceAllString(line, `"time":"…"`); got != want {
		t.Errorf("the event log holds\n%s\nwant\n%s", got, want)
	}

	got := read(t, filepath.Join(notesFolder(store), "01-orchestrator-to-lead.md"))
	if got != brief {
		t.Errorf("the brief holds %q, want %q", got, brief)
	}
}

func TestUpdateLogsEachHandoverWithItsNote(t *testing.T) {
	store := created(t)
	moves := []flow.Handover{
		{From: flow.RoleLead},
		{From: flow.RoleEngineer},
		{From: flow.RoleQA, Verdict: flow.VerdictApprove},
		{From: flow.RoleReviewer, Verdict: flow.VerdictReject},
	}

	for _, move := range moves {
		if _, err := store.Update(slug, noted(move.From, move.Verdict), hand(move)); err != nil {
			t.Fatal(err)
		}
	}

	type line struct {
		Kind    string
		Role    flow.Role
		Verdict flow.Verdict
		Note    string
		Holder  flow.Role
		Round   int
	}

	lead, engineer, qa, reviewer := flow.RoleLead, flow.RoleEngineer, flow.RoleQA, flow.RoleReviewer
	want := []line{
		{"start", flow.RoleOrchestrator, "", "01-orchestrator-to-lead.md", lead, 0},
		{"handover", lead, "", "02-lead-to-engineer.md", engineer, 1},
		{"handover", engineer, "", "03-engineer-to-qa.md", qa, 1},
		{"handover", qa, "approve", "04-qa-to-reviewer.md", reviewer, 1},
		{"handover", reviewer, "reject", "05-reviewer-to-engineer.md", engineer, 2},
	}

	logged := events(t, store)

	var got []line
	for _, event := range logged {
		got = append(got, line{
			event.Kind, event.Role, event.Verdict, event.Note, event.Flow.Holder, event.Flow.Round,
		})
	}

	if !slices.Equal(got, want) {
		t.Errorf("the log holds\n%+v\nwant\n%+v", got, want)
	}

	if !slices.IsSortedFunc(logged, func(a, b state.Event) int { return a.Time.Compare(b.Time) }) {
		t.Errorf("the log's times are out of order: %+v", logged)
	}

	note := read(t, filepath.Join(notesFolder(store), "04-qa-to-reviewer.md"))
	if note != "From the qa.\n" {
		t.Errorf("QA's note holds %q, want its text", note)
	}
}

func TestUpdateLogsAnEventWithoutANote(t *testing.T) {
	store := created(t)

	if _, err := store.Update(slug, logged(), keep); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSuffix(read(t, eventsFile(store)), "\n"), "\n")
	if len(lines) != 2 || strings.Contains(lines[1], `"note"`) ||
		strings.Contains(lines[1], `"verdict"`) || strings.Contains(lines[1], `"text"`) {
		t.Errorf("the event log holds %q, want a second event without a note, a verdict "+
			"or a text", lines)
	}

	wantNotes(t, store, "01-orchestrator-to-lead.md")
}

func TestUpdateNumbersANoteAfterTheLastOne(t *testing.T) {
	store := created(t)
	for _, name := range []string{"07-qa-to-reviewer.md", ".DS_Store", "draft.md", "x-12.md"} {
		writeFile(t, filepath.Join(notesFolder(store), name), "")
	}

	if _, err := store.Update(
		slug,
		noted(flow.RoleLead, ""),
		hand(flow.Handover{From: flow.RoleLead}),
	); err != nil {
		t.Fatal(err)
	}

	if got := events(t, store)[1].Note; got != "08-lead-to-engineer.md" {
		t.Errorf("the note is %s, want 08-lead-to-engineer.md", got)
	}

	if got := read(
		t,
		filepath.Join(notesFolder(store), "08-lead-to-engineer.md"),
	); got != "From the lead.\n" {
		t.Errorf("the note holds %q, want the lead's text", got)
	}
}

func TestEveryChangeLogsAnEvent(t *testing.T) {
	entries := map[string]state.Entry{
		"no event":         {},
		"no role":          {Event: state.Event{Kind: state.EventHandover}},
		"no kind":          {Event: state.Event{Role: flow.RoleLead}},
		"a note, no event": {Note: []byte("From the lead.\n")},
	}

	for name, entry := range entries {
		t.Run(name, func(t *testing.T) {
			store := created(t)
			before := read(t, featureFile(store))

			_, err := store.Update(slug, entry, hand(flow.Handover{From: flow.RoleLead}))
			wantError(t, err, "change feature demo-1: event ")
			wantError(t, err, "want its kind and its role")

			if after := read(t, featureFile(store)); after != before || len(events(t, store)) != 1 {
				t.Errorf("Update wrote a change without its event")
			}

			wantNotes(t, store, "01-orchestrator-to-lead.md")

			fresh := state.Store{Dir: t.TempDir()}
			wantError(t, fresh.Create(started(t), entry), "new feature demo-1: event ")
			wantNothingIn(t, fresh.Dir)
		})
	}
}

func TestUpdateLogsAnEventOnALineOfItsOwnAfterATornOne(t *testing.T) {
	store := created(t)
	torn := `{"time":"2026-10-31T18:00:00Z","event":"hand`

	log, err := os.OpenFile(eventsFile(store), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = log.WriteString(torn)
	if err := errors.Join(err, log.Close()); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Update(slug, logged(), keep); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(read(t, eventsFile(store)), "\n")
	if len(lines) != 4 || lines[1] != torn || lines[3] != "" {
		t.Fatalf("the event log holds %q, want the start, the torn line and the new event", lines)
	}

	var event state.Event
	if err := json.Unmarshal([]byte(lines[2]), &event); err != nil || event.Kind != "handover" {
		t.Errorf("the new event is %q (%v), want the handover", lines[2], err)
	}
}

package credits_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/credits"
)

// usageRecord returns the event that copilot logs for a model call that spent the nano AIU, with a
// few of the fields that Copilot CLI 1.0.105 gives it (fact 48).
func usageRecord(nano string) string {
	return `{"type":"session.usage_record","data":{"usage":{"model":"claude-opus-5.5",` +
		`"copilotUsage":{"totalNanoAiu":` + nano + `},"aiCreditsStatus":"complete"}},` +
		`"timestamp":"2026-11-05T12:00:00.000Z"}` + "\n"
}

// session writes the events into a session's folder of the test's own, and returns the folder.
func session(t *testing.T, events ...string) string {
	t.Helper()

	folder := t.TempDir()

	err := os.WriteFile(filepath.Join(folder, "events.jsonl"), []byte(strings.Join(events, "")),
		0o600)
	if err != nil {
		t.Fatal(err)
	}

	return folder
}

func TestSpentSumsTheUsageRecords(t *testing.T) {
	folder := session(t,
		`{"type":"session.start","data":{"sessionId":"1"}}`+"\n",
		usageRecord("250000000"),
		usageRecord("1500000000"),
		`{"type":"session.usage_checkpoint","data":{"totalNanoAiu":1750000000}}`+"\n",
		usageRecord("250000000"))

	if got, err := credits.Spent(folder); got != 2 || err != nil {
		t.Errorf("Spent = %v, %v; want 2 and no error", got, err)
	}
}

func TestSpentReadsTheRecordsWholeLinesAlone(t *testing.T) {
	cases := []struct {
		name   string
		events []string
		want   float64
	}{
		{"of none", nil, 0},
		{
			// Longer than a bufio.Reader's buffer, and a bufio.Scanner's longest line.
			"in a long line",
			[]string{strings.Replace(usageRecord("500000000"), `"timestamp"`,
				`"padding":"`+strings.Repeat("x", 100_000)+`","timestamp"`, 1)},
			0.5,
		},
		{
			"after a line that a crash cut short, which the next event finished",
			[]string{
				strings.TrimSuffix(usageRecord("1000000000"), "}\n") + usageRecord("500000000"),
				usageRecord("250000000"),
			},
			0.25,
		},
		{
			"but of another type",
			[]string{strings.Replace(usageRecord("1000000000"), "usage_record",
				"usage_record_total", 1)},
			0,
		},
		{
			"before the last line, which copilot is still writing",
			[]string{usageRecord("250000000"), strings.TrimSuffix(usageRecord("1000000000"), "\n")},
			0.25,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := credits.Spent(session(t, test.events...))
			if got != test.want || err != nil {
				t.Errorf("Spent = %v, %v; want %v and no error", got, err, test.want)
			}
		})
	}
}

func TestSpentFailsWithoutEvents(t *testing.T) {
	got, err := credits.Spent(filepath.Join(t.TempDir(), "none"))
	if got != 0 || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Spent = %v, %v; want 0 and an error that wraps fs.ErrNotExist", got, err)
	}
}

func TestSpentReportsEventsThatItCantRead(t *testing.T) {
	folder := t.TempDir()
	if err := os.Mkdir(filepath.Join(folder, "events.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := credits.Spent(folder)
	if got != 0 || err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Spent = %v, %v; want 0 and the error that reading a folder gives", got, err)
	}
}

func TestSpentReadsNoLinkOutOfTheFolder(t *testing.T) {
	outside, folder := session(t, usageRecord("1000000000")), t.TempDir()

	err := os.Symlink(filepath.Join(outside, "events.jsonl"), filepath.Join(folder, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := credits.Spent(folder)
	if got != 0 || err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf(
			"Spent = %v, %v; want 0 and the error that a link out of the folder gives",
			got,
			err,
		)
	}
}

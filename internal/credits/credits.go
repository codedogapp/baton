// Package credits reads the AI credits that a copilot session has spent from the events that
// copilot logs for it: events.jsonl in the session's folder, a JSON object on each line (facts 8
// and 48).
package credits

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"errors"
	"io"
	"os"
)

// eventsFile is the file in a session's folder in which copilot logs the session's events.
const eventsFile = "events.jsonl"

// recordType is the type of the event that copilot logs for each model call, with the AI credits
// that the call spent.
const recordType = "session.usage_record"

// nanoPerCredit is the number of nano AIU in an AI credit (fact 8).
const nanoPerCredit = 1e9

// record is what baton reads of an event: its type and, for a usage record, its usage.
type record struct {
	Type string `json:"type"`
	Data struct {
		Usage usage `json:"usage"`
	} `json:"data"`
}

// usage is what a usage record's model call spent: its AI credits, in nano AIU.
type usage struct {
	CopilotUsage struct {
		TotalNanoAiu float64 `json:"totalNanoAiu"`
	} `json:"copilotUsage"`
}

// Spent returns the AI credits that the session in the folder has spent so far: the sum of its
// usage records, one for each model call, which add up to the totals of copilot's own usage
// checkpoints, which come far less often (fact 48). It reads whole lines only, since copilot may
// be writing the last one, and skips a line that isn't an event, such as one that a crash cut
// short and the next event finished. It reads no link out of the folder, and fails with an error
// that wraps fs.ErrNotExist when the folder holds no events.
func Spent(folder string) (float64, error) {
	file, err := os.OpenInRoot(folder, eventsFile)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	nano, lines := 0.0, bufio.NewReader(file)

	for {
		line, err := lines.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nano / nanoPerCredit, nil
		}

		if err != nil {
			return 0, err
		}

		var event record
		if bytes.Contains(line, []byte(recordType)) && json.Unmarshal(line, &event) == nil &&
			event.Type == recordType {
			nano += event.Data.Usage.CopilotUsage.TotalNanoAiu
		}
	}
}

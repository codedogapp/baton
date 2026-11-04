package jira_test

import (
	"encoding/json/v2"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/jira"
)

func TestMarkdown(t *testing.T) {
	testdata := os.DirFS("testdata")

	golden, err := fs.Glob(testdata, "*.md")
	if err != nil || len(golden) == 0 {
		t.Fatalf("no tickets in testdata: %v", err)
	}

	for _, name := range golden {
		key := strings.TrimSuffix(name, ".md")
		t.Run(key, func(t *testing.T) {
			var ticket jira.Ticket
			decode(t, testdata, key+".json", &ticket)

			var page struct {
				Comments []jira.Comment `json:"comments"`
			}

			decode(t, testdata, key+".comments.json", &page)
			ticket.Comments = page.Comments

			want := string(read(t, testdata, name))
			if got := ticket.Markdown(); got != want {
				t.Errorf("Markdown() =\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// decode reads a file of testdata as JSON into v.
func decode(t *testing.T, testdata fs.FS, name string, v any) {
	t.Helper()

	if err := json.Unmarshal(read(t, testdata, name), v); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

// read returns a file of testdata.
func read(t *testing.T, testdata fs.FS, name string) []byte {
	t.Helper()

	data, err := fs.ReadFile(testdata, name)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

package adf_test

import (
	"encoding/json/v2"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/adf"
)

// TestMain puts the tests in a zone east of UTC, so that a date shown in the machine's zone, not
// in UTC, fails them, wherever they run.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("UTC+2", int((2 * time.Hour).Seconds()))

	m.Run()
}

// TestMarkdown renders each document in testdata and compares it with the Markdown beside it.
func TestMarkdown(t *testing.T) {
	testdata := os.DirFS("testdata")

	docs, err := fs.Glob(testdata, "*.json")
	if err != nil || len(docs) == 0 {
		t.Fatalf("no documents in testdata: %v", err)
	}

	for _, doc := range docs {
		name := strings.TrimSuffix(doc, ".json")
		t.Run(name, func(t *testing.T) {
			var node adf.Node
			if err := json.Unmarshal(read(t, testdata, doc), &node); err != nil {
				t.Fatal(err)
			}

			want := strings.TrimSuffix(string(read(t, testdata, name+".md")), "\n")
			if got := adf.Markdown(node); got != want {
				t.Errorf("Markdown() =\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestMarkdownOfOneNode(t *testing.T) {
	tests := []struct {
		name string
		node adf.Node
		want string
	}{
		{"nothing", adf.Node{}, ""},
		{"an inline node", adf.Node{Type: "text", Text: "plain"}, "plain"},
		{
			"an unknown node",
			adf.Node{Type: "future", Content: []adf.Node{{Type: "media"}}},
			"[attachment]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adf.Markdown(tt.node); got != tt.want {
				t.Errorf("Markdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func read(t *testing.T, files fs.FS, path string) []byte {
	t.Helper()

	data, err := fs.ReadFile(files, path)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

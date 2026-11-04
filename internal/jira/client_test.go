package jira_test

import (
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestTicket(t *testing.T) {
	cases := []struct {
		key, golden, cloudID string
		pages                int32
	}{
		{"DEMO-1", "DEMO-1", cloud, 2},
		{"demo-6", "DEMO-6", "{" + strings.ToUpper(cloud) + "}", 1},
	}

	for _, test := range cases {
		t.Run(test.key, func(t *testing.T) {
			server := &fake{tenant: tenantInfo(t, test.cloudID)}

			got, err := server.client(t).Ticket(t.Context(), test.key)
			if err != nil {
				t.Fatal(err)
			}

			want := string(read(t, os.DirFS("testdata"), test.golden+".md"))
			if got.Markdown() != want {
				t.Errorf("Markdown() =\n%s\nwant:\n%s", got.Markdown(), want)
			}

			if pages := server.pages.Load(); pages != test.pages {
				t.Errorf("read %d pages of comments, want %d", pages, test.pages)
			}
		})
	}
}

func TestTicketStopsAtAnEmptyPage(t *testing.T) {
	server := &fake{extra: 2}

	got, err := server.client(t).Ticket(t.Context(), "DEMO-1")
	if err != nil {
		t.Fatal(err)
	}

	if comments, pages := len(got.Comments), server.pages.Load(); comments != 3 || pages != 3 {
		t.Errorf("read %d comments on %d pages, want 3 on 3", comments, pages)
	}
}

func TestTicketReadsAnAnswerOfUpTo16MiB(t *testing.T) {
	head, tail := `{"key":"DEMO-1","fields":{"summary":"`, `"}}`
	summary := strings.Repeat("a", 16<<20-len(head)-len(tail))

	cases := []struct {
		name, summary string
		fails         bool
	}{
		{"16 MiB", summary, false},
		{"a byte more", summary + "a", true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := &fake{issue: func(w http.ResponseWriter, _ *http.Request) {
				// The client stops reading at its limit, so the rest of a longer answer may not
				// reach it.
				_, _ = io.WriteString(w, head+test.summary+tail)
			}}

			got, err := server.client(t).Ticket(t.Context(), "DEMO-1")
			if test.fails != (err != nil) || !test.fails && got.Fields.Summary != test.summary {
				t.Errorf("Ticket() = a summary of %d bytes, %v", len(got.Fields.Summary), err)
			}
		})
	}
}

func TestTicketRefusesAKeyThatIsNotOne(t *testing.T) {
	for _, key := range []string{
		"", "DEMO", "DEMO1", "DEMO-", "-1", "1DEMO-1", "_DEMO-1", "DEMO-1x", "x/demo-1",
		"DEMO-1/comment", "DEMO-1?fields=x", "DEMO-1 ",
	} {
		t.Run(key, func(t *testing.T) {
			server := &fake{}
			want := "want a ticket's key, such as DEMO-1, not " + strconv.Quote(key)

			_, err := server.client(t).Ticket(t.Context(), key)
			if err == nil || err.Error() != want {
				t.Errorf("Ticket() = %v, want %s", err, want)
			}

			if requests := server.requests.Load(); requests != 0 {
				t.Errorf("sent %d requests, want none", requests)
			}
		})
	}
}

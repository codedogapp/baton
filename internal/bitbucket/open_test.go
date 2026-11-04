package bitbucket_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/bitbucket"
)

// demo is a repo on Bitbucket Cloud.
var demo = repo("acme", "demo")

// repo returns the repo in the workspace with the slug.
func repo(workspace, slug string) bitbucket.Repo {
	return bitbucket.Repo{Workspace: workspace, Slug: slug}
}

func TestRepoOf(t *testing.T) {
	cases := []struct {
		key       string
		want      bitbucket.Repo
		bitbucket bool
	}{
		{"bitbucket.org/acme/demo", demo, true},
		{"Bitbucket.ORG/Acme/demo", repo("Acme", "demo"), true},
		{"bitbucket.org/acme/demo/more", repo("acme", "demo/more"), true},
		{"bitbucket.org", bitbucket.Repo{}, true},
		{"github.com/acme/demo", demo, false},
		{"bitbucket.org.example.com/acme/demo", demo, false},
		{"/Users/me/demo", repo("Users", "me/demo"), false},
	}

	for _, test := range cases {
		t.Run(test.key, func(t *testing.T) {
			got, ok := bitbucket.RepoOf(test.key)
			if got != test.want || ok != test.bitbucket {
				t.Errorf("RepoOf() = %+v, %t, want %+v, %t", got, ok, test.want, test.bitbucket)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	cases := []struct {
		name   string
		repo   bitbucket.Repo
		answer http.HandlerFunc
		line   string
	}{
		{
			"201 Created", demo, nil,
			"POST api.bitbucket.org/2.0/repositories/acme/demo/pullrequests",
		},
		{
			"200 OK",
			repo("Acme_1", "demo-web.v2"),
			func(w http.ResponseWriter, _ *http.Request) {
				answer(t, w, http.StatusOK, `{"links":{"html":{"href":"`+link+`"}}}`)
			},
			"POST api.bitbucket.org/2.0/repositories/Acme_1/demo-web.v2/pullrequests",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := &fake{answer: test.answer}
			pr := bitbucket.PullRequest{
				Repo: test.repo, Branch: "feature/demo-1", Title: "Add the demo",
				Description: "# Report\n\nDone.\n",
			}

			got, err := server.client(t).Open(t.Context(), pr)
			if got != link || err != nil {
				t.Errorf("Open() = %q, %v, want %q", got, err, link)
			}

			server.want(t, test.line, `{"title":"Add the demo","description":"# Report\n\n`+
				`Done.\n","source":{"branch":{"name":"feature/demo-1"}}}`)
		})
	}
}

func TestOpenRefusesARepoThatIsntOne(t *testing.T) {
	bad := []bitbucket.Repo{
		repo("", "demo"), repo("acme", ""), repo("acme", "demo/more"), repo("..", "demo"),
		repo("acme", "."), repo("ac me", "demo"), repo("acme", "demo?"),
	}

	for _, name := range bad {
		t.Run(name.String(), func(t *testing.T) {
			server := &fake{}

			_, err := server.client(t).Open(t.Context(), bitbucket.PullRequest{Repo: name})

			want := "want a Bitbucket repo's workspace and slug, such as acme/demo, not \"" +
				name.String() + "\""
			if err == nil || err.Error() != want {
				t.Errorf("Open() = %v, want %s", err, want)
			}

			server.none(t)
		})
	}
}

func TestOpenRefusesADescriptionThatIsntUTF8(t *testing.T) {
	server := &fake{}
	pr := bitbucket.PullRequest{Repo: demo, Description: "\xff"}

	_, err := server.client(t).Open(t.Context(), pr)
	if err == nil || !strings.HasPrefix(err.Error(), "open the pull request: write the request: ") {
		t.Errorf("Open() = %v, want an error that writes no request", err)
	}

	server.none(t)
}

func TestOpenFailsWithoutALink(t *testing.T) {
	const wantLink = "want the link to a pull request on Bitbucket, such as " +
		"https://bitbucket.org/acme/demo/pull-requests/1"

	cases := []struct {
		name, body, link string
	}{
		{"no link", `{"type":"pullrequest","id":7}`, ""},
		{
			"a repo's link", `{"links":{"html":{"href":"https://bitbucket.org/acme/demo"}}}`,
			"https://bitbucket.org/acme/demo",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := &fake{answer: answering(t, http.StatusCreated, test.body)}

			_, err := server.client(t).Open(t.Context(), bitbucket.PullRequest{Repo: demo})

			want := `open the pull request: Bitbucket opened it, but gave its link as "` +
				test.link + `": ` + wantLink
			if err == nil || err.Error() != want {
				t.Errorf("Open() = %v, want %s", err, want)
			}
		})
	}
}

func TestOpenReadsAnAnswerOfUpTo16MiB(t *testing.T) {
	head, tail := `{"links":{"html":{"href":"`+link+`"}},"summary":{"raw":"`, `"}}`
	raw := strings.Repeat("a", 16<<20-len(head)-len(tail))

	cases := []struct {
		name, raw string
		fails     bool
	}{
		{"16 MiB", raw, false},
		{"a byte more", raw + "a", true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := &fake{answer: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				// The client stops reading at its limit, so the rest of a longer answer may not
				// reach it.
				_, _ = io.WriteString(w, head+test.raw+tail)
			}}

			got, err := server.client(t).Open(t.Context(), bitbucket.PullRequest{Repo: demo})
			if test.fails != (err != nil) || !test.fails && got != link {
				t.Errorf("Open() = %q, %v", got, err)
			}
		})
	}
}

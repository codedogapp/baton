package cli_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/keychain"
)

// The tests' Jira site, the user's account there and the password that the keychain holds for it,
// and the site's cloud ID.
const (
	jiraConfig = `{"jira": {"site": "example.atlassian.net", "email": "me@example.com"}}`
	stored     = "jira-api-1234"
	issue      = "https://api.atlassian.com/ex/jira/0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0" +
		"/rest/api/3/issue/DEMO-1"
)

// jiraAnswers are the answers of the stand-in for the site and Atlassian's gateway, by URL.
var jiraAnswers = map[string]string{
	"https://example.atlassian.net/_edge/tenant_info": `{
		"cloudId": "0F1E2D3C-4B5A-6978-8796-A5B4C3D2E1F0"}`,
	issue + "?fields=summary,issuetype,status,parent,description,subtasks,issuelinks": `{
		"key": "DEMO-1",
		"fields": {
			"summary": "Show the ticket\u001b[2J",
			"issuetype": {"name": "Task"},
			"status": {"name": "To Do"},
			"description": {"type": "doc", "version": 1, "content": [{"type": "paragraph",
				"content": [{"type": "text", "text":
					"No bell\u0007,\r\nno \u009b31mcolour\u001b[0m, but tabs\tand ünïcödé."}]}]}
		}
	}`,
	issue + "/comment?orderBy=created&startAt=0": `{"comments": [{"author": {"displayName":
		"Ann\u0000"}, "created": "2026-11-04T10:00:00.000+0100", "body": {"type": "doc",
		"version": 1, "content": [{"type": "paragraph", "content": [{"type": "text",
		"text": "Done\u007f."}]}]}}], "total": 1}`,
}

// shown is the ticket that baton jira prints, without the control characters of its text.
const shown = `# DEMO-1: Show the ticket[2J

- Type: Task
- Status: To Do

## Description

No bell,
no 31mcolour[0m, but tabs	and ünïcödé.

## Comments

### Ann, 2026-11-04T10:00:00.000+0100

Done.
`

// jiraStandIn answers a GET with its answer by URL, if the request carries the login that the
// gateway gets, and that the site doesn't, or else as Jira does.
type jiraStandIn struct{}

// RoundTrip answers the request.
func (jiraStandIn) RoundTrip(req *http.Request) (*http.Response, error) {
	status, body := http.StatusOK, jiraAnswers[req.URL.String()]
	user, password, login := req.BasicAuth()

	switch {
	case login != (req.URL.Host == "api.atlassian.com") ||
		login && (user != "me@example.com" || password != stored):
		status, body = http.StatusUnauthorized, `{"code": 401, "message": "Unauthorized"}`
	case body == "":
		status, body = http.StatusNotFound, `{"errorMessages": ["Issue does not exist or you `+
			`do not have permission to see it."], "errors": {}}`
	}

	return &http.Response{
		StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}, nil
}

// keychainFunc reads a password from the keychain, as cli.Env.Keychain does.
type keychainFunc = func(ctx context.Context, service, account string) (string, error)

// holding returns a stand-in for the keychain that holds the password for baton's Jira item of
// the user's account, and no other item.
func holding(password string) keychainFunc {
	return func(_ context.Context, service, account string) (string, error) {
		if service != "baton-jira" || account != "me@example.com" {
			return "", keychain.ErrNotFound
		}

		return password, nil
	}
}

// jiraIn returns the environment of baton jira, with the arguments after its name, in a repo of
// its own, with the config file's content, the keychain and the stand-in for Jira.
func jiraIn(t *testing.T, config string, read keychainFunc, args ...string) cli.Env {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	writeConfig(t, config)

	return cli.Env{
		Args:     append([]string{"jira"}, args...),
		Getenv:   func(string) string { return "" },
		Dir:      repoNamed(t, "main"),
		HTTP:     &http.Client{Transport: jiraStandIn{}},
		Keychain: read,
	}
}

// TestJiraPrintsTheTicket prints the ticket, with its key in any case, as Markdown, without the
// control characters of its text but newlines and tabs, so none reaches the terminal. The site
// gets no login, and the gateway gets the email from the config file and the keychain's password.
func TestJiraPrintsTheTicket(t *testing.T) {
	got := runEnv(t.Context(), jiraIn(t, jiraConfig, holding(stored), "demo-1"))
	if want := (result{stdout: shown}); got != want {
		t.Errorf("baton jira = %+v, want %+v", got, want)
	}
}

// refusedLogin is the error of baton jira when Jira refuses the login as the email, which the
// command in it has quoted for the shell.
func refusedLogin(email, quoted string) string {
	return "the Jira login as " + email + " failed (read DEMO-1: 401 Unauthorized: " +
		"Unauthorized): check that email, and if the token has expired or lacks the scope " +
		"read:jira-work, make a new one and replace it with `security add-generic-password -U " +
		"-s baton-jira -a " + quoted + " -w`, which asks for it"
}

// TestJiraRefuses refuses a command line that names no key, or more than one, or a text that isn't
// a key, before it reads the keychain; a repo without a Jira site and the user's email there, and
// a user without a token in the keychain; it reports the keychain's failure, and Jira's, with the
// command that replaces a token for a login that Jira refuses. A case without a config or a
// keychain has the tests' own.
func TestJiraRefuses(t *testing.T) {
	noSite := `no Jira site for this repo: set "jira" in {file} to the site's host name and ` +
		`your email address at Atlassian, such as {"site": "example.atlassian.net", ` +
		`"email": "you@example.com"}`

	cases := map[string]struct {
		config string
		read   keychainFunc
		args   []string
		want   string
	}{
		"without a key":    {want: "want a ticket's key, such as DEMO-1"},
		"with a blank key": {args: []string{" "}, want: "want a ticket's key, such as DEMO-1"},
		"with two keys": {
			args: []string{"DEMO-1", "DEMO-2"},
			want: `unexpected argument "DEMO-2": quote each text for the shell`,
		},
		"with a text that isn't a key": {
			read: func(context.Context, string, string) (string, error) {
				return "", errors.New("read the keychain for a text that isn't a key")
			},
			args: []string{"DEMO"}, want: `want a ticket's key, such as DEMO-1, not "DEMO"`,
		},
		"without Jira's settings": {config: `{}`, args: []string{"DEMO-1"}, want: noSite},
		"without the email": {
			config: `{"jira": {"site": "example.atlassian.net"}}`, args: []string{"DEMO-1"},
			want: noSite,
		},
		"without the site": {
			config: `{"jira": {"email": "me@example.com"}}`, args: []string{"DEMO-1"},
			want: noSite,
		},
		"with a site that isn't one": {
			config: `{"jira": {"site": "https://example.atlassian.net"}}`,
			args:   []string{"DEMO-1"},
			want: `config file {file}: jira: site "https://example.atlassian.net": want the ` +
				`site's host name, such as example.atlassian.net`,
		},
		"without the token": {
			config: `{"jira": {"site": "example.atlassian.net", "email": "o'brien@example.com"}}`,
			read: func(context.Context, string, string) (string, error) {
				return "", keychain.ErrNotFound
			},
			args: []string{"DEMO-1"},
			want: "no Jira API token for o'brien@example.com in the keychain: add it with " +
				"`security add-generic-password -s baton-jira -a 'o'\\''brien@example.com' " +
				"-w`, which asks for it",
		},
		"when the keychain fails": {
			read: func(context.Context, string, string) (string, error) {
				return "", errors.New("read the keychain: exit status 36")
			},
			args: []string{"DEMO-1"},
			want: "read the keychain: exit status 36",
		},
		"with the wrong token": {
			read: holding("jira-api-0000"), args: []string{"DEMO-1"},
			want: refusedLogin("me@example.com", "'me@example.com'"),
		},
		"with an email that Jira doesn't know": {
			config: `{"jira": {"site": "example.atlassian.net", "email": "o'brien@example.com"}}`,
			read: func(context.Context, string, string) (string, error) {
				return stored, nil
			},
			args: []string{"DEMO-1"},
			want: refusedLogin("o'brien@example.com", `'o'\''brien@example.com'`),
		},
		"with a ticket that Jira doesn't show": {
			args: []string{"DEMO-9"},
			want: "read DEMO-9: 404 Not Found: Issue does not exist or you do not have " +
				"permission to see it.",
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			read := test.read
			if read == nil {
				read = holding(stored)
			}

			got := runEnv(t.Context(), jiraIn(t, cmp.Or(test.config, jiraConfig), read,
				test.args...))

			want := strings.ReplaceAll(test.want, "{file}", configPath(t))
			if want := (result{code: 1, stderr: "baton jira: " + want + "\n"}); got != want {
				t.Errorf("baton jira = %+v, want %+v", got, want)
			}
		})
	}
}

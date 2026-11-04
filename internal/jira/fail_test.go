package jira_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/jira"
)

func TestTicketFails(t *testing.T) {
	refuse := func(status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}
	}

	cases := []struct {
		name, key, token string
		server           *fake
		want             string
	}{
		{
			"a site without Jira", "DEMO-1", scoped,
			&fake{tenant: refuse(http.StatusNotFound, "<html>Not here</html>")},
			"read the cloud ID of example.atlassian.net: 404 Not Found",
		},
		{
			"a wrong token", "DEMO-1", "wrong", &fake{},
			"read DEMO-1: 401 Unauthorized: Unauthorized",
		},
		{
			"a ticket that isn't there", "DEMO-99", scoped, &fake{},
			"read DEMO-99: 404 Not Found: Issue does not exist or you do not have permission " +
				"to see it.",
		},
		{
			"a proxy's page", "DEMO-1", scoped,
			&fake{issue: refuse(http.StatusBadGateway, "<html>Bad gateway</html>")},
			"read DEMO-1: 502 Bad Gateway",
		},
		{
			"Jira's messages and the gateway's", "DEMO-1", scoped,
			&fake{issue: refuse(http.StatusBadRequest,
				`{"errorMessages":["One."," Two. "],"message":"Three."}`)},
			"read DEMO-1: 400 Bad Request: One.  Two.  Three.",
		},
		{
			"refused comments", "DEMO-1", scoped,
			&fake{comments: refuse(http.StatusBadRequest, `{"errorMessages":["Bad order."]}`)},
			"read the comments on DEMO-1: 400 Bad Request: Bad order.",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := test.server.client(t)
			client.Token = test.token

			_, err := client.Ticket(t.Context(), test.key)
			if err == nil || err.Error() != test.want {
				t.Errorf("Ticket() = %v, want %s", err, test.want)
			}
		})
	}
}

func TestTicketFailsOnAWrongAnswer(t *testing.T) {
	cases := []struct {
		name   string
		server *fake
		want   string
	}{
		{
			"a cloud ID that isn't one",
			&fake{tenant: func(w http.ResponseWriter, _ *http.Request) {
				answer(t, w, http.StatusOK, `{"cloudId":"example"}`)
			}},
			"read the cloud ID of example.atlassian.net: invalid uuid",
		},
		{
			"a ticket that isn't JSON",
			&fake{issue: func(w http.ResponseWriter, _ *http.Request) {
				answer(t, w, http.StatusOK, "<html>A ticket</html>")
			}},
			"read DEMO-1: ",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.server.client(t).Ticket(t.Context(), "DEMO-1")
			if err == nil || !strings.HasPrefix(err.Error(), test.want) {
				t.Errorf("Ticket() = %v, want an error that starts with %q", err, test.want)
			}
		})
	}
}

// errOffline is the error of a transport without a network.
var errOffline = errors.New("offline")

type offline struct{}

func (offline) RoundTrip(*http.Request) (*http.Response, error) { return nil, errOffline }

func TestTicketFailsToConnect(t *testing.T) {
	cases := []struct {
		name string
		site string
		want func(error) bool
	}{
		{"offline", site, func(err error) bool { return errors.Is(err, errOffline) }},
		{"a site that isn't a host", "example atlassian.net", func(err error) bool {
			return strings.Contains(err.Error(), "invalid character")
		}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := jira.Client{HTTP: &http.Client{Transport: offline{}}, Site: test.site}

			if _, err := client.Ticket(t.Context(), "DEMO-1"); err == nil || !test.want(err) {
				t.Errorf("Ticket() = %v", err)
			}
		})
	}
}

func TestTicketStopsWhenItsContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	server := &fake{}

	if _, err := server.client(t).Ticket(ctx, "DEMO-1"); !errors.Is(err, context.Canceled) {
		t.Errorf("Ticket() = %v, want %v", err, context.Canceled)
	}
}

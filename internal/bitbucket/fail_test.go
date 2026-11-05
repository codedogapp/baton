package bitbucket_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/bitbucket"
)

func TestOpenFails(t *testing.T) {
	cases := []struct {
		name, token string
		answer      http.HandlerFunc
		want        string
	}{
		{"a wrong token", "expired", nil, "401 Unauthorized: Unauthorized"},
		{
			"a message and a detail", token,
			answering(t, http.StatusBadRequest, `{"type":"error","error":{"message":"Bad `+
				`request","detail":" There are no changes to be pulled "}}`),
			"400 Bad Request: Bad request: There are no changes to be pulled",
		},
		{
			"a detail that isn't text", token,
			answering(t, http.StatusForbidden, `{"type":"error","error":{"message":"Your `+
				`credentials lack one or more required privilege scopes.","detail":`+
				`{"required":["write:pullrequest:bitbucket"]}}}`),
			"403 Forbidden: Your credentials lack one or more required privilege scopes.: " +
				`{"required":["write:pullrequest:bitbucket"]}`,
		},
		{
			"a proxy's page", token,
			answering(t, http.StatusBadGateway, "<html>Bad gateway</html>"), "502 Bad Gateway",
		},
		{
			"an empty message", token,
			answering(t, http.StatusInternalServerError, `{"type":"error","error":`+
				`{"message":" ","detail":null}}`),
			"500 Internal Server Error",
		},
		{
			"300 Multiple Choices", token,
			answering(t, http.StatusMultipleChoices, "{}"), "300 Multiple Choices",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := (&fake{answer: test.answer}).client(t)
			client.Token = test.token

			_, err := client.Open(t.Context(), bitbucket.PullRequest{Repo: demo})

			want := "open the pull request: " + test.want
			if err == nil || err.Error() != want {
				t.Errorf("Open() = %v, want %s", err, want)
			}
		})
	}
}

func TestOpenFailsOnARefusalOfMoreThan16MiB(t *testing.T) {
	server := &fake{answer: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		// The client stops reading at its limit, so the rest of the answer may not reach it.
		_, _ = io.WriteString(w, `{"type":"error","error":{"message":"`+
			strings.Repeat("a", 16<<20)+`"}}`)
	}}

	_, err := server.client(t).Open(t.Context(), bitbucket.PullRequest{Repo: demo})
	if want := "open the pull request: 400 Bad Request"; err == nil || err.Error() != want {
		t.Errorf("Open() = %.100s, want %s", err, want)
	}
}

func TestOpenFailsOnAnAnswerThatIsntJSON(t *testing.T) {
	server := &fake{answer: answering(t, http.StatusCreated, "<html>Created</html>")}

	_, err := server.client(t).Open(t.Context(), bitbucket.PullRequest{Repo: demo})
	if err == nil || !strings.HasPrefix(err.Error(), "open the pull request: jsontext: ") {
		t.Errorf("Open() = %v, want an error of JSON's", err)
	}
}

func TestCommentFails(t *testing.T) {
	server := &fake{answer: answering(t, http.StatusNotFound, `{"type":"error","error":`+
		`{"message":"No such pull request"}}`)}

	err := server.client(t).Comment(t.Context(), link, "# Report")

	want := `comment on the pull request "` + link + `": 404 Not Found: No such pull request`
	if err == nil || err.Error() != want {
		t.Errorf("Comment() = %v, want %s", err, want)
	}
}

// TestLoginError checks that a 401 to the user's email and token is a LoginError, which a caller
// can tell apart, and that another refusal isn't.
func TestLoginError(t *testing.T) {
	forbidden := answering(t, http.StatusForbidden, `{"type":"error","error":{"message":"No"}}`)

	cases := map[string]struct {
		token   string
		answer  http.HandlerFunc
		comment bool
		login   bool
	}{
		"for a pull request":      {token: "expired", login: true},
		"for a comment":           {token: "expired", comment: true, login: true},
		"for a 403":               {token: token, answer: forbidden},
		"for a comment's 403":     {token: token, answer: forbidden, comment: true},
		"for a pull request's OK": {token: token},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			client := (&fake{answer: test.answer}).client(t)
			client.Token = test.token

			var err error
			if test.comment {
				err = client.Comment(t.Context(), link, "# Report")
			} else {
				_, err = client.Open(t.Context(), bitbucket.PullRequest{Repo: demo})
			}

			if _, refused := errors.AsType[bitbucket.LoginError](err); refused != test.login {
				t.Errorf("got %v, a LoginError: %t, want %t", err, refused, test.login)
			}
		})
	}
}

// errOffline is the error of a transport without a network.
var errOffline = errors.New("offline")

type offline struct{}

func (offline) RoundTrip(*http.Request) (*http.Response, error) { return nil, errOffline }

func TestOpenFailsToConnect(t *testing.T) {
	client := bitbucket.Client{HTTP: &http.Client{Transport: offline{}}}

	if _, err := client.Open(t.Context(), bitbucket.PullRequest{Repo: demo}); !errors.Is(err,
		errOffline) {
		t.Errorf("Open() = %v, want %v", err, errOffline)
	}
}

func TestOpenFailsWithoutAContext(t *testing.T) {
	var none context.Context

	server := &fake{}

	_, err := server.client(t).Open(none, bitbucket.PullRequest{Repo: demo})
	if err == nil || !strings.HasSuffix(err.Error(), "nil Context") {
		t.Errorf("Open() = %v, want an error for the nil context", err)
	}

	server.none(t)
}

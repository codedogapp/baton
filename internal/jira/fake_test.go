package jira_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/codedogapp/baton/internal/jira"
)

// The made-up site and its cloud ID, and the user's email and token, which only the gateway gets.
const (
	site   = "example.atlassian.net"
	cloud  = "0f1e2d3c-4b5a-4987-8a6b-5c4d3e2f1a0b"
	email  = "user@example.com"
	scoped = "scoped-api-token"
)

// pageSize is the most comments that the fake gives on a page, so that DEMO-1's three take two.
const pageSize = 2

// fake stands in for Jira: the site, which gives its cloud ID, and the gateway, which gives the
// user the tickets in testdata and their comments. A test can set a handler of its own in place of
// the fake's, and comments that the pages count but don't hold.
type fake struct {
	tenant, issue, comments http.HandlerFunc
	extra                   int

	requests, pages atomic.Int32
}

// client returns a client of the fake, as the user, whose requests reach the fake whatever their
// host.
func (f *fake) client(t *testing.T) jira.Client {
	t.Helper()

	issue := "GET api.atlassian.com/ex/jira/" + cloud + "/rest/api/3/issue/{key}"
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+site+"/_edge/tenant_info", or(f.tenant, tenantInfo(t, cloud)))
	mux.HandleFunc(issue, gateway(or(f.issue, ticket(t))))
	mux.HandleFunc(issue+"/comment", gateway(or(f.comments, f.page(t))))

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	transport := toServer{addr: server.Listener.Addr().String(), next: server.Client().Transport}

	return jira.Client{
		HTTP:  &http.Client{Transport: transport},
		Site:  site,
		Email: email,
		Token: scoped,
	}
}

// or returns the test's handler, if it set one, or else the fake's.
func or(handler, fallback http.HandlerFunc) http.HandlerFunc {
	if handler != nil {
		return handler
	}

	return fallback
}

// tenantInfo answers as the site's tenant_info, with the cloud ID, and fails the test if the site
// gets the user's login.
func tenantInfo(t *testing.T, cloudID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("the site got the user's login")
		}

		answer(t, w, http.StatusOK, `{"cloudId":"`+cloudID+`"}`)
	}
}

// gateway refuses a request without the user's login, or that doesn't take JSON, as the gateway
// would, and passes the others on.
func gateway(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()

		switch {
		case !ok || user != email || pass != scoped:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":401,"message":"Unauthorized"}`)
		case r.Header.Get("Accept") != "application/json":
			w.WriteHeader(http.StatusNotAcceptable)
		default:
			next(w, r)
		}
	}
}

// ticket answers with a ticket in testdata, if the request names baton's fields, or as Jira
// answers for a ticket that isn't there. It fails the test if the key isn't in capitals, as
// Jira's keys are, which a file system that ignores case wouldn't.
func ticket(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		want := "summary,issuetype,status,parent,description,subtasks,issuelinks"
		if got := r.URL.Query().Get("fields"); got != want {
			t.Errorf("fields = %q, want %q", got, want)
		}

		key := r.PathValue("key")
		if key != strings.ToUpper(key) {
			t.Errorf("asked for %s, not in capitals", key)
		}

		data, err := fs.ReadFile(os.DirFS("testdata"), key+".json")
		if err != nil {
			answer(t, w, http.StatusNotFound, `{"errorMessages":["Issue does not exist or you `+
				`do not have permission to see it."],"errors":{}}`)

			return
		}

		answer(t, w, http.StatusOK, string(data))
	}
}

// page answers with a page of a ticket's comments in testdata, oldest first, from the request's
// startAt, and fails the test if the client asks for more pages than there are. It runs on the
// server's goroutine, so it fails the test with t.Errorf, not t.Fatal.
func (f *fake) page(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var all struct {
			Comments []jsontext.Value `json:"comments"`
		}

		data, err := fs.ReadFile(os.DirFS("testdata"), r.PathValue("key")+".comments.json")
		if err == nil {
			err = json.Unmarshal(data, &all)
		}

		query := r.URL.Query()
		start, atoi := strconv.Atoi(query.Get("startAt"))

		switch {
		case f.pages.Add(1) > 5:
			fail(t, w, "the client asked for more pages than there are")
		case err != nil:
			fail(t, w, "read the comments: %v", err)
		case atoi != nil || start > len(all.Comments) || query.Get("orderBy") != "created":
			fail(t, w, "asked for comments with %s", r.URL.RawQuery)
		default:
			comments := all.Comments[start:min(start+pageSize, len(all.Comments))]
			total := len(all.Comments) + f.extra

			page, err := json.Marshal(map[string]any{"comments": comments, "total": total})
			if err != nil {
				fail(t, w, "write a page: %v", err)

				return
			}

			answer(t, w, http.StatusOK, string(page))
		}
	}
}

// fail fails the test, and answers the client with an error.
func fail(t *testing.T, w http.ResponseWriter, format string, args ...any) {
	t.Helper()
	t.Errorf(format, args...)
	w.WriteHeader(http.StatusInternalServerError)
}

// answer writes the fake's answer.
func answer(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()

	w.WriteHeader(status)

	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("answer: %v", err)
	}
}

// toServer sends each request to the test server, whatever its host, which stays in the request's
// Host header for the server's routes.
type toServer struct {
	addr string
	next http.RoundTripper
}

func (s toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Host = s.addr

	return s.next.RoundTrip(req)
}

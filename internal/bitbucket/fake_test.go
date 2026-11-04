package bitbucket_test

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/codedogapp/baton/internal/bitbucket"
)

// The user's email and token, which only Bitbucket's API gets, and the link to the pull request
// that the fake opens.
const (
	email = "user@example.com"
	token = "api-token"
	link  = "https://bitbucket.org/acme/demo/pull-requests/7"
)

// fake stands in for Bitbucket Cloud's API, for the user: it opens the pull request at link, in
// any repo, and takes comments on any pull request. It keeps each request that it gets. A test
// can set an answer of its own in place of the fake's.
type fake struct {
	answer http.HandlerFunc

	mu  sync.Mutex
	got []request
}

// request is a request that the fake got: its method, host and path, and its JSON body.
type request struct {
	line string
	body any
}

// client returns a client of the fake, as the user, whose requests reach the fake whatever their
// host.
func (f *fake) client(t *testing.T) bitbucket.Client {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.serve(t, w, r)
	}))
	t.Cleanup(server.Close)

	transport := toServer{addr: server.Listener.Addr().String(), next: server.Client().Transport}

	return bitbucket.Client{HTTP: &http.Client{Transport: transport}, Email: email, Token: token}
}

// serve keeps the request, then refuses it without the user's login, or unless it sends and
// takes JSON, as Bitbucket would, and answers the others.
func (f *fake) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	var body any
	if err := json.UnmarshalRead(r.Body, &body); err != nil {
		t.Errorf("read the request's body: %v", err)
	}

	f.mu.Lock()
	f.got = append(f.got, request{line: r.Method + " " + r.Host + r.URL.Path, body: body})
	f.mu.Unlock()

	user, pass, ok := r.BasicAuth()

	switch {
	case !ok || user != email || pass != token:
		answer(t, w, http.StatusUnauthorized, `{"type":"error","error":{"message":"Unauthorized"}}`)
	case r.Header.Get("Content-Type") != "application/json":
		w.WriteHeader(http.StatusUnsupportedMediaType)
	case r.Header.Get("Accept") != "application/json":
		w.WriteHeader(http.StatusNotAcceptable)
	case f.answer != nil:
		f.answer(w, r)
	case strings.HasSuffix(r.URL.Path, "/comments"):
		answer(t, w, http.StatusCreated, `{"type":"pullrequest_comment","id":1}`)
	default:
		answer(t, w, http.StatusCreated, `{"type":"pullrequest","id":7,"links":{"html":`+
			`{"href":"`+link+`"}}}`)
	}
}

// want fails the test unless the fake got one request alone, with the line and the JSON body.
func (f *fake) want(t *testing.T, line, body string) {
	t.Helper()

	var want any
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatal(err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.got) != 1 || f.got[0].line != line || !reflect.DeepEqual(f.got[0].body, want) {
		t.Errorf("the fake got %v, want {%s %v}", f.got, line, want)
	}
}

// none fails the test if the fake got a request.
func (f *fake) none(t *testing.T) {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.got) != 0 {
		t.Errorf("the fake got %v, want no request", f.got)
	}
}

// answering returns a handler that answers with the status and the body.
func answering(t *testing.T, status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		answer(t, w, status, body)
	}
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
// Host header.
type toServer struct {
	addr string
	next http.RoundTripper
}

func (s toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Host = s.addr

	return s.next.RoundTrip(req)
}

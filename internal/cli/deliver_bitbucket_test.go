package cli_test

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

const (
	// bitbucketLink is the link to the pull request that the stand-in for Bitbucket opens.
	bitbucketLink = "https://bitbucket.org/acme/demo/pull-requests/7"
	// storedForBitbucket is the user's API token for Bitbucket, in the stand-in keychain.
	storedForBitbucket = "bitbucket-api-1234"
	// bitbucketConfig is the config file that names the user's account on Bitbucket Cloud.
	bitbucketConfig = `{"bitbucket": {"email": "me@example.com"}}`
	// pullRequests is the address of acme/demo's pull requests in Bitbucket's API.
	pullRequests = "https://api.bitbucket.org/2.0/repositories/acme/demo/pullrequests"
)

// bitbucketStandIn answers each request with its status and its body, as Bitbucket's API does,
// and keeps the requests.
type bitbucketStandIn struct {
	status int
	body   string
	got    []bitbucketRequest
}

// bitbucketRequest is a request that the stand-in for Bitbucket got: its method and URL, the
// user and the password of its login, and its body, decoded from JSON.
type bitbucketRequest struct {
	method, url, user, password string
	fields                      any
}

// RoundTrip answers the request.
func (b *bitbucketStandIn) RoundTrip(req *http.Request) (*http.Response, error) {
	defer func() { _ = req.Body.Close() }()

	var fields any
	if err := json.UnmarshalRead(req.Body, &fields); err != nil {
		return nil, err
	}

	user, password, _ := req.BasicAuth()
	b.got = append(b.got, bitbucketRequest{req.Method, req.URL.String(), user, password, fields})

	return &http.Response{
		StatusCode: b.status, Status: fmt.Sprintf("%d %s", b.status, http.StatusText(b.status)),
		Body: io.NopCloser(strings.NewReader(b.body)), Request: req,
	}, nil
}

// onBitbucket returns a repo where demo-1 is ready to deliver, whose origin's URL names the repo
// acme/demo on Bitbucket Cloud, with the config file's content. Its pushes go to the test's bare
// repo, so that none leaves the machine.
func onBitbucket(t *testing.T, origin, config string) delivery {
	t.Helper()

	ready := readyToDeliver(t)
	gitIn(t, ready.mainDir, exec.CommandContext(t.Context(), "git", "remote", "remove", "origin"))
	write(t, filepath.Join(ready.mainDir, ".git"), "remotes", "[remote \"origin\"]\n\turl = "+
		origin+"\n\tpushurl = "+ready.origin+"\n")
	gitIn(t, ready.mainDir, exec.CommandContext(t.Context(), "git", "config", "include.path",
		"remotes"))
	writeConfig(t, config)

	return ready
}

// withToken returns a stand-in for the keychain that holds the token in baton's Bitbucket item of
// the user's account, and no other item.
func withToken() keychainFunc {
	return holdingItem("baton-bitbucket", "me@example.com", storedForBitbucket)
}

// bitbucketDelivery returns the environment of baton deliver demo-1 in the orchestrator's pane,
// with the arguments after the slug, in the repo, with the keychain and the stand-in for
// Bitbucket.
func bitbucketDelivery(ready delivery, pulls *bitbucketStandIn, read keychainFunc,
	args ...string,
) cli.Env {
	return cli.Env{
		Args:     append([]string{"deliver", slug}, args...),
		Getenv:   func(name string) string { return orchestratorPane()[name] },
		Dir:      ready.mainDir,
		HTTP:     &http.Client{Transport: pulls},
		Keychain: read,
	}
}

// wantRequests checks the requests that the stand-in for Bitbucket got.
func wantRequests(t *testing.T, pulls *bitbucketStandIn, want ...bitbucketRequest) {
	t.Helper()

	if !reflect.DeepEqual(pulls.got, want) {
		t.Errorf("Bitbucket got %+v; want %+v", pulls.got, want)
	}
}

// TestDeliverOpensThePullRequestOnBitbucket delivers a feature of a repo on Bitbucket Cloud, by
// either form of its origin's URL: it pushes the branch, then opens its pull request with
// Bitbucket's API, as the user, with the lead's final report as its description, without gh.
func TestDeliverOpensThePullRequestOnBitbucket(t *testing.T) {
	for _, origin := range []string{
		"git@bitbucket.org:acme/demo.git", "https://me@Bitbucket.org/acme/demo",
	} {
		t.Run(origin, func(t *testing.T) {
			ready := onBitbucket(t, origin, bitbucketConfig)
			calls := standInGh(t, "exit 1\n")
			pulls := &bitbucketStandIn{status: http.StatusCreated, body: `{"id": 7, "links": ` +
				`{"html": {"href": "` + bitbucketLink + `"}}}`}

			got := runEnv(t.Context(), bitbucketDelivery(ready, pulls, withToken(), "--title",
				" Add the demo \n"))
			if want := (result{stdout: "Delivered demo-1: pushed linked to origin and opened " +
				"its pull request, " + bitbucketLink + ".\n"}); got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			wantPushed(t, ready.origin, head(t, ready.mainDir))
			wantGh(t, calls, "")
			wantRequests(t, pulls, bitbucketRequest{
				http.MethodPost, pullRequests, "me@example.com", storedForBitbucket,
				map[string]any{
					"title": "Add the demo", "description": "From the lead.\n",
					"source": map[string]any{"branch": map[string]any{"name": "linked"}},
				},
			})

			feature, err := ready.store.Read(slug)
			if err != nil || feature.Flow.Status != flow.StatusDelivered ||
				feature.PullRequest != bitbucketLink {
				t.Errorf("Read = %+v, %v; want it delivered, with the pull request", feature, err)
			}
		})
	}
}

// TestDeliverCommentsOnThePullRequestOnBitbucket comments the lead's new report on the pull
// request that an earlier delivery opened on Bitbucket Cloud.
func TestDeliverCommentsOnThePullRequestOnBitbucket(t *testing.T) {
	ready := onBitbucket(t, "git@bitbucket.org:acme/demo.git", bitbucketConfig)
	edit(t, ready.store, func(feature *state.Feature) { feature.PullRequest = bitbucketLink })

	calls := standInGh(t, "exit 1\n")
	pulls := &bitbucketStandIn{status: http.StatusCreated, body: `{"id": 1}`}

	got := runEnv(t.Context(), bitbucketDelivery(ready, pulls, withToken()))
	if want := (result{stdout: "Delivered demo-1: pushed linked to origin and commented the " +
		"lead's report on its pull request, " + bitbucketLink + ".\n"}); got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}

	wantPushed(t, ready.origin, head(t, ready.mainDir))
	wantGh(t, calls, "")
	wantRequests(t, pulls, bitbucketRequest{
		http.MethodPost, pullRequests + "/7/comments", "me@example.com", storedForBitbucket,
		map[string]any{"content": map[string]any{"raw": "From the lead.\n"}},
	})

	if feature, err := ready.store.Read(slug); err != nil ||
		feature.Flow.Status != flow.StatusDelivered || feature.PullRequest != bitbucketLink {
		t.Errorf("Read = %+v, %v; want it delivered, with the pull request", feature, err)
	}
}

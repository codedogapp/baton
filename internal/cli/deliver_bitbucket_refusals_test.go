package cli_test

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestDeliverRefusesWithoutABitbucketLogin refuses a feature of a repo on Bitbucket Cloud
// without the user's account in the config file, or their token in the keychain, before the
// push: nothing leaves the machine, and baton's folder stays as it was. A case without a config
// or a keychain has the tests' own.
func TestDeliverRefusesWithoutABitbucketLogin(t *testing.T) {
	cases := []struct {
		name   string
		config string
		read   keychainFunc
		setup  func(t *testing.T)
		want   string // The reason, after "baton deliver: ", with {file} for the config file.
	}{
		{
			name: "without the account", config: `{}`,
			want: `no Bitbucket account for this repo: set "bitbucket" in {file} to your email ` +
				`address at Atlassian, such as {"email": "you@example.com"}`,
		},
		{
			name: "with an email that isn't one", config: `{"bitbucket": {"email": "me"}}`,
			want: `config file {file}: bitbucket: email "me": want your email address at ` +
				`Atlassian alone`,
		},
		{
			name: "without the token", config: `{"bitbucket": {"email": "o'brien@example.com"}}`,
			want: "no Bitbucket API token for o'brien@example.com in the keychain: add it with " +
				"`security add-generic-password -s baton-bitbucket -a 'o'\\''brien@example.com' " +
				"-w`, which asks for it",
		},
		{
			name: "when the keychain fails",
			read: func(context.Context, string, string) (string, error) {
				return "", errors.New("read the keychain: exit status 36")
			},
			want: "read the keychain: exit status 36",
		},
		{
			name: "when git can't read origin's URL",
			setup: func(t *testing.T) {
				gitBut(t, "remote) echo 'fatal: bad config' >&2; exit 128 ;;")
			},
			want: "find the origin remote: fatal: bad config",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ready := onBitbucket(t, "git@bitbucket.org:acme/demo.git",
				cmp.Or(test.config, bitbucketConfig))
			if test.setup != nil {
				test.setup(t)
			}

			calls, pulls := standInGh(t, "exit 1\n"), &bitbucketStandIn{status: http.StatusCreated}
			before := snapshot(t, ready.store.Dir)

			read := test.read
			if read == nil {
				read = withToken()
			}

			got := runEnv(t.Context(), bitbucketDelivery(ready, pulls, read, "--title", "Add it"))

			want := strings.ReplaceAll(test.want, "{file}", configPath(t))
			if want := (result{code: 1, stderr: "baton deliver: " + want + "\n"}); got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			wantPushed(t, ready.origin, "")
			wantGh(t, calls, "")
			wantRequests(t, pulls)

			if after := snapshot(t, ready.store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed:\n%v\nwant:\n%v", after, before)
			}
		})
	}
}

// refusedBitbucketLogin is the error of baton deliver when Bitbucket refuses the user's login,
// with the reason.
func refusedBitbucketLogin(reason string) string {
	return "the Bitbucket login as me@example.com failed (" + reason + "): check that email, " +
		"and if the token has expired, make a new one with the scopes " +
		"read:pullrequest:bitbucket and write:pullrequest:bitbucket, and replace it with " +
		"`security add-generic-password -U -s baton-bitbucket -a 'me@example.com' -w`, which " +
		"asks for it"
}

// bitbucketFailure is a delivery to Bitbucket Cloud that fails after the push.
type bitbucketFailure struct {
	link   string // The pull request of an earlier delivery, if any.
	status int    // The stand-in's answer, with the body.
	body   string
	gone   bool   // Whether the lead's report is gone.
	want   string // The reason, after "baton deliver: ", with {report} for the report.
}

// ready returns a repo where demo-1 is ready for the delivery, and its arguments after the slug.
func (f bitbucketFailure) ready(t *testing.T) (delivery, []string) {
	t.Helper()

	ready := onBitbucket(t, "git@bitbucket.org:acme/demo.git", bitbucketConfig)
	if f.gone {
		if err := os.Remove(ready.report); err != nil {
			t.Fatal(err)
		}
	}

	if f.link == "" {
		return ready, []string{"--title", "Add it"}
	}

	edit(t, ready.store, func(feature *state.Feature) { feature.PullRequest = f.link })

	return ready, nil
}

// TestDeliverReportsWhatBitbucketRefuses checks that a delivery whose pull request Bitbucket
// refuses, or whose report baton can't read, after the push, leaves the feature ready, with the
// pull request of an earlier delivery, if any, so that a later delivery tries again.
func TestDeliverReportsWhatBitbucketRefuses(t *testing.T) {
	const scopes = `{"required":["write:pullrequest:bitbucket"]}`

	cases := map[string]bitbucketFailure{
		"a login that Bitbucket refuses": {
			status: http.StatusUnauthorized,
			want:   refusedBitbucketLogin("open the pull request: 401 Unauthorized"),
		},
		"a comment's login that Bitbucket refuses": {
			link: bitbucketLink, status: http.StatusUnauthorized,
			body: `{"type": "error", "error": {"message": "Unauthorized"}}`,
			want: refusedBitbucketLogin(`comment on the pull request "` + bitbucketLink +
				`": 401 Unauthorized: Unauthorized`),
		},
		"a token without the scope": {
			status: http.StatusForbidden,
			body: `{"type": "error", "error": {"message": "Your credentials lack one or more ` +
				`required privilege scopes.", "detail": ` + scopes + `}}`,
			want: "open the pull request: 403 Forbidden: Your credentials lack one or more " +
				"required privilege scopes.: " + scopes,
		},
		"a report that is gone": {
			status: http.StatusCreated, gone: true,
			want: "read the lead's report: open {report}: no such file or directory",
		},
		"a comment's report that is gone": {
			link: bitbucketLink, status: http.StatusCreated, gone: true,
			want: "read the lead's report: open {report}: no such file or directory",
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			ready, args := test.ready(t)
			calls := standInGh(t, "exit 1\n")
			pulls := &bitbucketStandIn{status: test.status, body: test.body}

			got := runEnv(t.Context(), bitbucketDelivery(ready, pulls, withToken(), args...))

			want := strings.ReplaceAll(test.want, "{report}", ready.report)
			if want := (result{code: 1, stderr: "baton deliver: " + want + "\n"}); got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			wantPushed(t, ready.origin, head(t, ready.mainDir))
			wantGh(t, calls, "")

			if (len(pulls.got) == 1) == test.gone {
				t.Errorf("Bitbucket got %+v", pulls.got)
			}

			if feature, err := ready.store.Read(slug); err != nil ||
				feature.Flow.Status != flow.StatusReady || feature.PullRequest != test.link {
				t.Errorf("Read = %+v, %v; want it ready, with the pull request %q", feature, err,
					test.link)
			}
		})
	}
}

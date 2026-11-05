package bitbucket_test

import (
	"strconv"
	"testing"
)

func TestComment(t *testing.T) {
	server := &fake{}

	if err := server.client(t).Comment(t.Context(), link, "# Report\n\nAgain.\n"); err != nil {
		t.Errorf("Comment() = %v", err)
	}

	server.want(t, "POST api.bitbucket.org/2.0/repositories/acme/demo/pullrequests/7/comments",
		`{"content":{"raw":"# Report\n\nAgain.\n"}}`)
}

func TestCommentRefusesALinkThatIsntAPullRequest(t *testing.T) {
	const (
		notLink = "want the link to a pull request on Bitbucket, such as " +
			"https://bitbucket.org/acme/demo/pull-requests/1"
		notRepo = "want a Bitbucket repo's workspace and slug, such as acme/demo, not "
	)

	cases := map[string]string{
		"https://github.com/acme/demo/pull/7":                  notLink,
		"http://bitbucket.org/acme/demo/pull-requests/7":       notLink,
		"xhttps://bitbucket.org/acme/demo/pull-requests/7":     notLink,
		"https://bitbucketxorg/acme/demo/pull-requests/7":      notLink,
		"https://bitbucket.org/acme/demo/pull-requests/7/":     notLink,
		"https://bitbucket.org/acme/demo/pull-requests/7x":     notLink,
		"https://bitbucket.org/acme/demo/pull-requests/7?a=b":  notLink,
		"https://bitbucket.org/acme/demo/pull-requests/07":     notLink,
		"https://bitbucket.org/acme/demo/pull-requests/0":      notLink,
		"https://bitbucket.org/acme/demo/pull-requests/":       notLink,
		"https://bitbucket.org/acme/demo/pull-request/7":       notLink,
		"https://bitbucket.org/acme/demo/more/pull-requests/7": notLink,
		"https://bitbucket.org/acme/pull-requests/7":           notLink,
		"https://bitbucket.org/../demo/pull-requests/7":        notRepo + `"../demo"`,
		"https://bitbucket.org/acme/de%2Fmo/pull-requests/7":   notRepo + `"acme/de%2Fmo"`,
	}

	for address, want := range cases {
		t.Run(address, func(t *testing.T) {
			server := &fake{}

			err := server.client(t).Comment(t.Context(), address, "# Report")

			want = "comment on the pull request " + strconv.Quote(address) + ": " + want
			if err == nil || err.Error() != want {
				t.Errorf("Comment() = %v, want %s", err, want)
			}

			server.none(t)
		})
	}
}

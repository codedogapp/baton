package config_test

import (
	"testing"

	"github.com/codedogapp/baton/internal/config"
)

func TestLoadLaysTheRepoOverTheFileForBitbucket(t *testing.T) {
	const content = `{
		"bitbucket": {"email": "me@example.com"},
		"repos": {
			"bitbucket.org/acme/demo": {"bitbucket": {"email": "o'brien+bb@example.co.uk"}},
			"bitbucket.org/acme/tap": {"bitbucket": {"email": ""}}
		}
	}`

	for given, want := range map[string]string{
		"git@bitbucket.org:acme/demo.git":    "o'brien+bb@example.co.uk",
		"https://bitbucket.org/acme/tap":     "me@example.com",
		"https://bitbucket.org/acme/another": "me@example.com",
	} {
		t.Run(given, func(t *testing.T) {
			if got := load(t, content, given).Bitbucket; got != (config.Bitbucket{Email: want}) {
				t.Errorf("Load gives the Bitbucket settings %+v, want the email %q", got, want)
			}
		})
	}
}

func TestLoadRefusesABitbucketEmailThatIsntOne(t *testing.T) {
	const email = ": want your email address at Atlassian alone"

	for content, want := range map[string]string{
		`{"bitbucket": {"email": "me"}}`: `bitbucket: email "me"` + email,
		`{"bitbucket": {"email": "Me <me@example.com>"}}`: `bitbucket: email ` +
			`"Me <me@example.com>"` + email,
		`{"bitbucket": {"email": "me@example.com "}}`: `bitbucket: email "me@example.com "` + email,
		`{"repos": {"bitbucket.org/a/a": {"bitbucket": {"email": "me@"}}}}`: "repos: " +
			`bitbucket.org/a/a: bitbucket: email "me@"` + email,
		`{"jira": {"email": "me"}, "bitbucket": {"email": "me"}}`: `jira: email "me"` + email,
		`{"bitbucket": {"email": "me"}, "roles": {"lead": {"model": "-l"}}}`: `bitbucket: email ` +
			`"me"` + email,
	} {
		t.Run(want, func(t *testing.T) {
			dir := withFile(t, content)

			_, err := config.Load(dir, remote)
			if err == nil {
				t.Fatalf("Load took %s", content)
			}

			if want := "config file " + filePath(dir) + ": " + want; err.Error() != want {
				t.Errorf("Load says %q, want %q", err, want)
			}
		})
	}
}

package config_test

import (
	"testing"

	"github.com/codedogapp/baton/internal/config"
)

func TestLoadLaysTheRepoOverTheFileForJira(t *testing.T) {
	const content = `{
		"jira": {"site": "example.atlassian.net", "email": "me@example.com"},
		"repos": {
			"` + repo + `": {"jira": {"site": "My-Team.atlassian.net"}},
			"github.com/codedogapp/nudge": {"jira": {"email": "o'brien+jira@example.co.uk"}},
			"github.com/codedogapp/tap": {"jira": {"site": "a", "email": ""}}
		}
	}`

	for given, want := range map[string]config.Jira{
		remote: {Site: "My-Team.atlassian.net", Email: "me@example.com"},
		"https://github.com/codedogapp/nudge": {
			Site: "example.atlassian.net", Email: "o'brien+jira@example.co.uk",
		},
		"https://github.com/codedogapp/tap":  {Site: "a", Email: "me@example.com"},
		"https://github.com/codedogapp/demo": {Site: "example.atlassian.net", Email: "me@example.com"},
	} {
		t.Run(given, func(t *testing.T) {
			if got := load(t, content, given).Jira; got != want {
				t.Errorf("Load gives the Jira settings %+v, want %+v", got, want)
			}
		})
	}
}

func TestLoadRefusesJiraSettingsThatBatonCantUse(t *testing.T) {
	const site = ": want the site's host name, such as example.atlassian.net"

	const email = ": want your email address at Atlassian alone"

	for content, want := range map[string]string{
		`{"jira": {"site": "https://example.atlassian.net"}}`: `jira: site ` +
			`"https://example.atlassian.net"` + site,
		`{"jira": {"site": "example.atlassian.net/"}}`:    `jira: site "example.atlassian.net/"` + site,
		`{"jira": {"site": "example.atlassian.net/x"}}`:   `jira: site "example.atlassian.net/x"` + site,
		`{"jira": {"site": "example.atlassian.net:443"}}`: `jira: site "example.atlassian.net:443"` + site,
		`{"jira": {"site": "me@example.atlassian.net"}}`:  `jira: site "me@example.atlassian.net"` + site,
		`{"jira": {"site": "-x.atlassian.net"}}`:          `jira: site "-x.atlassian.net"` + site,
		`{"jira": {"site": "example.atlassian.net."}}`:    `jira: site "example.atlassian.net."` + site,
		`{"jira": {"site": " "}}`:                         `jira: site " "` + site,
		`{"jira": {"email": "me"}}`:                       `jira: email "me"` + email,
		`{"jira": {"email": "Me <me@example.com>"}}`:      `jira: email "Me <me@example.com>"` + email,
		`{"jira": {"email": " me@example.com"}}`:          `jira: email " me@example.com"` + email,
		`{"jira": {"site": "a b", "email": "me"}}`:        `jira: site "a b"` + site,
		`{"repos": {"github.com/a/a": {"jira": {"email": "me@"}}}}`: "repos: github.com/a/a: " +
			`jira: email "me@"` + email,
		`{"jira": {"email": "me"}, "roles": {"lead": {"model": "-l"}}}`: `jira: email "me"` + email,
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

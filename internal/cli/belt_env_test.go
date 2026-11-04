package cli_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// outward are the characters that start a URL that leaves the machine, in baton's push rules
// (fact 38).
const outward = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789["

// belt returns the variables that block a role's pushes and gh, after the count of git's settings
// in baton's environment, with git's settings for the repo's remotes: a push rule for each
// character that starts a URL that leaves the machine, one that keeps file:// URLs local, then
// the remotes'; and gh's config folder, which can't be there (decision 55).
func belt(count int, remotes ...[2]string) []string {
	rules := make([][2]string, 0, len(outward)+1+len(remotes))
	for _, char := range outward {
		rules = append(rules, pushRule(string(char)))
	}

	rules = append(rules, [2]string{"url.file://.pushInsteadOf", "file://"})
	rules = append(rules, remotes...)

	vars := make([]string, 0, 2*len(rules)+2)
	for i, rule := range rules {
		vars = append(vars, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", count+i, rule[0]),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", count+i, rule[1]))
	}

	return append(vars, fmt.Sprintf("GIT_CONFIG_COUNT=%d", count+len(rules)),
		"GH_CONFIG_DIR=/dev/null/baton-blocks-gh")
}

// pushRule returns git's setting, and its value, that sends a push to a URL that starts with the
// prefix to baton's missing remote helper.
func pushRule(prefix string) [2]string {
	return [2]string{"url.baton-blocks-push::" + prefix + ".pushInsteadOf", prefix}
}

// TestRolePaneAddsToGitsSettings has the lead's copilot get the belt's settings after git's own in
// baton's environment, in place of those past their count: a push rule for each remote's url that
// leaves the machine, and for the pushurls that leave, one empty pushurl for their remote, in a
// git that clears pushurls, as 2.46 does, or else a rule for each; none for local URLs (decision
// 55).
func TestRolePaneAddsToGitsSettings(t *testing.T) {
	cases := []struct {
		version string
		pushURL [][2]string // The belt's settings for origin's pushurls.
	}{
		{version: "git version 2.46.0", pushURL: [][2]string{{"remote.origin.pushurl", ""}}},
		{version: "git version 2.45.2", pushURL: [][2]string{
			{
				"url.baton-blocks-push::git@example.invalid:one.git.insteadOf",
				"git@example.invalid:one.git",
			},
			{
				"url.baton-blocks-push::git@example.invalid:two.git.insteadOf",
				"git@example.invalid:two.git",
			},
		}},
	}

	for _, test := range cases {
		t.Run(test.version, func(t *testing.T) {
			copilot, cache := standIn(t)
			gitBut(t, "version) echo '"+test.version+"'; exit ;;")
			setenv(t, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=user.name", "GIT_CONFIG_VALUE_0=user",
				"GIT_CONFIG_KEY_1=core.editor", "GIT_CONFIG_VALUE_1=vi")

			mainDir, linked := repos(t)
			write(t, filepath.Join(mainDir, ".git"), "remotes", `[remote "origin"]
	url = https://example.invalid/demo.git
	pushurl = git@example.invalid:one.git
	pushurl = git@example.invalid:two.git
[remote "local"]
	url = ../local.git
	pushurl = /srv/local.git
[remote "sibling"]
	url = sibling.git
`)
			gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "config", "include.path",
				"remotes"))

			store := started(t, mainDir, fresh())

			remotes := append([][2]string{pushRule("https://example.invalid/demo.git")},
				test.pushURL...)

			synctest.Test(t, func(t *testing.T) {
				env, _ := inPane(t, linked, paneOf(flow.RoleLead))

				wantCopilot(t, paneFor(t, env), flow.RoleLead, execed{
					path: copilot,
					args: copilotLine(flow.RoleLead, pluginIn(t, cache), leadsBrief(store)),
					env:  roleEnv(belt(1, remotes...)...),
				})
			})
		})
	}
}

// TestRolePaneMovesTheGitHubToken has the lead's copilot go without the GitHub tokens that gh and
// other tools read, and get the one that copilot would have read from them, GH_TOKEN's, or else
// GITHUB_TOKEN's, as its own, unless it has one (fact 38).
func TestRolePaneMovesTheGitHubToken(t *testing.T) {
	cases := []struct {
		name    string
		tokens  []string // The user's variables.
		copilot []string // Copilot's own token, if baton gives it one.
	}{
		{
			name: "every token",
			tokens: []string{
				"GH_TOKEN=gh", "GITHUB_TOKEN=github", "GH_ENTERPRISE_TOKEN=ghe",
				"GITHUB_ENTERPRISE_TOKEN=github-enterprise",
			},
			copilot: []string{"COPILOT_GITHUB_TOKEN=gh"},
		},
		{
			name:    "GITHUB_TOKEN's",
			tokens:  []string{"GH_TOKEN=", "GITHUB_TOKEN=github"},
			copilot: []string{"COPILOT_GITHUB_TOKEN=github"},
		},
		{name: "copilot's own", tokens: []string{"COPILOT_GITHUB_TOKEN=copilot", "GH_TOKEN=gh"}},
		{
			name:    "copilot's own, empty",
			tokens:  []string{"COPILOT_GITHUB_TOKEN=", "GH_TOKEN=gh"},
			copilot: []string{"COPILOT_GITHUB_TOKEN=gh"},
		},
		{name: "an enterprise's", tokens: []string{"GH_ENTERPRISE_TOKEN=ghe"}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			copilot, cache := standIn(t)
			setenv(t, test.tokens...)

			mainDir, linked := repos(t)
			store := started(t, mainDir, fresh())

			synctest.Test(t, func(t *testing.T) {
				env, _ := inPane(t, linked, paneOf(flow.RoleLead))

				wantCopilot(t, paneFor(t, env), flow.RoleLead, execed{
					path: copilot,
					args: copilotLine(flow.RoleLead, pluginIn(t, cache), leadsBrief(store)),
					env:  roleEnv(append(belt(0), test.copilot...)...),
				})
			})
		})
	}
}

// setenv sets the variables, each one name=value, for the test.
func setenv(t *testing.T, vars ...string) {
	t.Helper()

	for _, variable := range vars {
		name, value, _ := strings.Cut(variable, "=")
		t.Setenv(name, value)
	}
}

// leadsBrief returns the lead's first prompt in a new feature, with the orchestrator's brief.
func leadsBrief(store state.Store) string {
	return "Picked up demo-1 as lead: round 0 of 3, from orchestrator.\nNote: " +
		notePath(store, "01-orchestrator-to-lead.md") + "\n"
}

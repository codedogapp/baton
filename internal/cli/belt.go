package cli

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/codedogapp/baton/internal/repo"
)

// The belt is a role's second belt, after baton's guard: settings in the environment of the role's
// copilot, which every command and script there inherits, so that git's pushes out of the machine
// and gh fail, even where the guard can't see them (decision 55).

// blocked is the remote helper that git's push rules send a push to: one that doesn't exist, so
// that git refuses the push, with no network, and names baton's belt (fact 38).
const blocked = "baton-blocks-push::"

// outward are the characters that start a URL that leaves the machine, such as https://host/repo,
// host:repo or [::1]:repo, but no absolute path, and no ./ or ../ path (fact 38).
const outward = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789["

// ghBlocked is gh's config folder in a role's pane: one under a file, which can't be there, so that
// gh fails before it starts, its auth commands too, and never reaches the user's login (fact 38).
const ghBlocked = "/dev/null/baton-blocks-gh"

// tokens are the variables that give gh and other tools a GitHub token, which a role's pane goes
// without (decision 55).
var tokens = []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"}

// setting is a setting of git's, a name and its value.
type setting struct{ key, value string }

// belted returns a role's environment for copilot, from the folder: baton's, with the variables,
// and with the belt (decision 55).
func belted(ctx context.Context, dir string, vars ...string) ([]string, error) {
	count, err := gitCount()
	if err != nil {
		return nil, err
	}

	rules, err := pushRules(ctx, dir)
	if err != nil {
		return nil, err
	}

	vars = slices.Concat(vars, gitVars(count, rules), ghVars())

	return slices.DeleteFunc(environment(vars...), func(variable string) bool {
		name, _, _ := strings.Cut(variable, "=")

		return slices.Contains(tokens, name)
	}), nil
}

// gitCount returns the count of git's settings in baton's environment, which the belt's follow. It
// refuses one that git reads but Go doesn't, such as " 1"; with a negative one, git fails itself.
func gitCount() (int, error) {
	text := os.Getenv("GIT_CONFIG_COUNT")
	if text == "" {
		return 0, nil
	}

	count, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("GIT_CONFIG_COUNT=%q isn't a count of git's settings", text)
	}

	return count, nil
}

// pushRules returns git's settings that block a push out of the machine, from the folder: a
// pushInsteadOf rule for each character that starts such a URL, and one that keeps file:// URLs
// local; one for each remote's url that leaves, which beats the user's own shorter rules; and for
// each remote's pushurl that leaves, which those rules skip, an empty pushurl, which clears the
// remote's pushurls, so that the push goes to its url, or, in a git that can't clear them, an
// insteadOf rule, which blocks fetches from URLs that start with the pushurl too (fact 38).
func pushRules(ctx context.Context, dir string) ([]setting, error) {
	urls, err := repo.RemoteURLs(ctx, dir)
	if err != nil {
		return nil, err
	}

	clears, err := repo.ClearsPushURLs(ctx)
	if err != nil {
		return nil, err
	}

	rules := make([]setting, 0, len(outward)+1+len(urls))
	for _, char := range outward {
		rules = append(rules, blockPush(string(char)))
	}

	rules = append(rules, setting{"url.file://.pushInsteadOf", "file://"})

	for _, url := range urls {
		switch {
		case !leaves(url.URL):
		case !url.Push:
			rules = append(rules, blockPush(url.URL))
		case !clears:
			rules = append(rules, setting{"url." + blocked + url.URL + ".insteadOf", url.URL})
		case !slices.Contains(rules, setting{"remote." + url.Remote + ".pushurl", ""}):
			rules = append(rules, setting{"remote." + url.Remote + ".pushurl", ""})
		}
	}

	return rules, nil
}

// blockPush returns git's rule that sends a push to a URL that starts with the prefix to the
// blocked remote helper.
func blockPush(prefix string) setting {
	return setting{"url." + blocked + prefix + ".pushInsteadOf", prefix}
}

// leaves reports whether git takes the URL for one that leaves the machine: one with a colon before
// any slash, as git reads it, but no file:// URL (fact 38).
func leaves(url string) bool {
	before, _, found := strings.Cut(url, ":")

	return found && !strings.Contains(before, "/") && !strings.HasPrefix(url, "file://")
}

// gitVars returns the variables that give git the settings, after the count of those in baton's
// environment.
func gitVars(count int, settings []setting) []string {
	vars := make([]string, 0, 2*len(settings)+1)
	for i, each := range settings {
		vars = append(vars, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", count+i, each.key),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", count+i, each.value))
	}

	return append(vars, "GIT_CONFIG_COUNT="+strconv.Itoa(count+len(settings)))
}

// ghVars returns the variables that block gh, and that give copilot the token that it would have
// taken from GH_TOKEN or GITHUB_TOKEN, which the role's pane goes without, unless it has its own
// (fact 38).
func ghVars() []string {
	vars := []string{"GH_CONFIG_DIR=" + ghBlocked}

	if os.Getenv("COPILOT_GITHUB_TOKEN") != "" {
		return vars
	}

	if token := cmp.Or(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")); token != "" {
		vars = append(vars, "COPILOT_GITHUB_TOKEN="+token)
	}

	return vars
}

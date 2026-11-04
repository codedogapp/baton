package config

import (
	"errors"
	"fmt"
	"maps"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/codedogapp/baton/internal/flow"
)

// efforts are the reasoning efforts that copilot takes (fact 31).
var efforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

// minCredits is the least AI credits cap that copilot takes (fact 10).
const minCredits = 30

// check returns an error for the first setting in the file that baton can't use, with the repos'
// sections in the order of their keys.
func check(top Settings, repos map[string]Settings) error {
	if err := top.check(); err != nil {
		return err
	}

	for _, name := range slices.Sorted(maps.Keys(repos)) {
		switch {
		case name == "":
			return errors.New("repos: an empty key: want a remote's URL")
		case key(name) != name:
			return fmt.Errorf("repos: %q: want %q, the URL's host and path", name, key(name))
		}

		if err := repos[name].check(); err != nil {
			return fmt.Errorf("repos: %s: %w", name, err)
		}
	}

	return nil
}

// check returns an error for the first setting that baton can't use, with the roles in the order
// of their names.
func (s Settings) check() error {
	switch {
	case s.RoundCap < 0:
		return fmt.Errorf("roundCap %d: want 1 or more", s.RoundCap)
	case s.Gate != "" && strings.TrimSpace(s.Gate) == "", strings.HasPrefix(s.Gate, "-"):
		// sh would read a dash at the start as one of its options.
		return fmt.Errorf("gate %q: want the command that checks the repo's code", s.Gate)
	}

	if err := s.Jira.check(); err != nil {
		return fmt.Errorf("jira: %w", err)
	}

	for _, role := range slices.Sorted(maps.Keys(s.Roles)) {
		if err := s.Roles[role].check(role); err != nil {
			return fmt.Errorf("roles: %s: %w", role, err)
		}
	}

	return nil
}

// check returns an error for the first of the role's settings that baton can't use. A value that
// starts with a dash would read as one of copilot's flags.
func (r Role) check(role flow.Role) error {
	switch {
	case !role.Known():
		return errors.New("want lead, engineer, qa, reviewer or orchestrator")
	case strings.HasPrefix(r.Model, "-"):
		return fmt.Errorf("model %q: want a model's name", r.Model)
	case r.ReasoningEffort != "" && !slices.Contains(efforts, r.ReasoningEffort):
		return fmt.Errorf("reasoningEffort %q: want one of %s", r.ReasoningEffort,
			strings.Join(efforts, ", "))
	case r.MaxAICredits != 0 && r.MaxAICredits < minCredits:
		return fmt.Errorf("maxAiCredits %g: want %d or more, or 0 for no cap", r.MaxAICredits,
			minCredits)
	}

	return checkTools(role, r.ExtraTools)
}

// hostName matches a host's name, such as example.atlassian.net: letters, digits, dots and dashes,
// with a letter or a digit at each end, so no scheme, user, port or path.
var hostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// check returns an error for the first Jira setting that baton can't use.
func (j Jira) check() error {
	switch {
	case j.Site != "" && !hostName.MatchString(j.Site):
		return fmt.Errorf("site %q: want the site's host name, such as example.atlassian.net",
			j.Site)
	case j.Email != "" && !address(j.Email):
		return fmt.Errorf("email %q: want your email address at Atlassian alone", j.Email)
	}

	return nil
}

// address reports whether the text is an email address alone, without a name or spaces.
func address(text string) bool {
	parsed, err := mail.ParseAddress(text)

	return err == nil && parsed.Address == text
}

// checkTools returns an error for extra tools that the role can't have: only the orchestrator gets
// tools of the user's (decision 10).
func checkTools(role flow.Role, tools []string) error {
	if tools != nil && role != flow.RoleOrchestrator {
		return errors.New("extraTools: only the orchestrator gets tools of the user's")
	}

	for _, tool := range tools {
		if tool == "" || strings.HasPrefix(tool, "-") {
			return fmt.Errorf("extraTools: %q: want a tool's name", tool)
		}
	}

	return nil
}

// key returns a repo's key in the config file, from its remote's URL: the URL's host and path,
// without its scheme, user and port, or a last slash or .git, so that a repo's HTTPS and SSH URLs
// give the same key, such as github.com/codedogapp/baton. A local path stays a path.
func key(remote string) string {
	name := remote

	if parsed, err := url.Parse(remote); err == nil && strings.Contains(remote, "://") {
		name = parsed.Hostname() + parsed.Path
	} else if host, path, ok := strings.Cut(remote, ":"); ok && !strings.Contains(host, "/") {
		// git reads [user@]host:path as an SSH URL, when no slash comes before the colon.
		if _, after, found := strings.CutLast(host, "@"); found {
			host = after
		}

		name = host + "/" + strings.TrimLeft(path, "/")
	}

	return strings.TrimSuffix(strings.TrimSuffix(name, "/"), ".git")
}

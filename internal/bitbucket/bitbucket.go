// Package bitbucket opens a feature's pull request on Bitbucket Cloud, and comments on it, through
// Bitbucket's REST API, as the user, with their email and an API token (decision 64).
package bitbucket

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Host is Bitbucket Cloud's host, which its repos' URLs name.
const Host = "bitbucket.org"

// api is the address of the repos in Bitbucket Cloud's REST API, the one address that gets the
// user's token (decision 64).
const api = "https://api.bitbucket.org/2.0/repositories/"

// maxAnswer is the most of an answer that baton reads, so that a wrong answer can't fill its
// memory.
const maxAnswer = 16 << 20

// namePattern matches a workspace's ID or a repo's slug, as Bitbucket makes them: letters, digits,
// dots, dashes and underscores. With dots alone refused too, a name can't change the address that
// it goes into.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// linkPattern matches the link to a pull request: its repo's workspace and slug, and its number.
var linkPattern = regexp.MustCompile(
	`^https://bitbucket\.org/([^/]+)/([^/]+)/pull-requests/([1-9][0-9]*)$`)

// Repo is a repo on Bitbucket Cloud: its workspace's ID and its slug, as its URLs name them.
type Repo struct{ Workspace, Slug string }

// RepoOf returns the repo that a remote's key names, such as bitbucket.org/acme/demo: the remote's
// URL without its scheme, user, port and .git, as the config file keys it (decision 49). It
// reports whether the key's host is Bitbucket Cloud's.
func RepoOf(key string) (Repo, bool) {
	host, path, _ := strings.Cut(key, "/")
	workspace, slug, _ := strings.Cut(path, "/")

	return Repo{Workspace: workspace, Slug: slug}, strings.EqualFold(host, Host)
}

// String returns the repo's workspace and slug, as its URLs name them, such as acme/demo.
func (r Repo) String() string {
	return r.Workspace + "/" + r.Slug
}

// check returns an error for a repo whose workspace or slug isn't a name that Bitbucket makes.
func (r Repo) check() error {
	for _, name := range []string{r.Workspace, r.Slug} {
		if !namePattern.MatchString(name) || strings.Trim(name, ".") == "" {
			return fmt.Errorf("want a Bitbucket repo's workspace and slug, such as acme/demo, "+
				"not %q", r.String())
		}
	}

	return nil
}

// Client opens pull requests on Bitbucket Cloud, and comments on them, as the user, with their
// Atlassian account's email and an API token that reads and writes pull requests (fact 47).
type Client struct {
	HTTP  *http.Client
	Email string
	Token string
}

// PullRequest is a pull request to open in the repo: from the branch, which the repo has, with the
// title, and the description in Markdown.
type PullRequest struct {
	Repo                       Repo
	Branch, Title, Description string
}

// page is a link to a page on Bitbucket's site, as its answers give one, such as a pull request's.
type page struct {
	Href string `json:"href"`
}

// LoginError is the error for an answer of 401 Unauthorized to the user's email and token, which
// Bitbucket gives for a wrong email, or a token that has expired (fact 47).
type LoginError struct{ error }

// Open opens the pull request, into its repo's main branch, and returns its link.
func (c Client) Open(ctx context.Context, request PullRequest) (string, error) {
	if err := request.Repo.check(); err != nil {
		return "", err
	}

	fields := map[string]any{
		"title":       request.Title,
		"description": request.Description,
		"source":      map[string]any{"branch": map[string]string{"name": request.Branch}},
	}

	var created struct {
		Links struct {
			HTML page `json:"html"`
		} `json:"links"`
	}

	address := api + request.Repo.String() + "/pullrequests"
	if err := c.post(ctx, address, fields, &created); err != nil {
		return "", fmt.Errorf("open the pull request: %w", err)
	}

	// A later delivery comments on the pull request at its link, so it must be one.
	link := created.Links.HTML.Href
	if _, _, err := parseLink(link); err != nil {
		return "", fmt.Errorf("open the pull request: Bitbucket opened it, but gave its "+
			"link as %q: %w", link, err)
	}

	return link, nil
}

// Comment comments the text, in Markdown, on the pull request at the link, which Open gave.
func (c Client) Comment(ctx context.Context, link, text string) error {
	repo, number, err := parseLink(link)
	if err != nil {
		return fmt.Errorf("comment on the pull request %q: %w", link, err)
	}

	fields := map[string]any{"content": map[string]string{"raw": text}}
	address := api + repo.String() + "/pullrequests/" + number + "/comments"

	if err := c.post(ctx, address, fields, nil); err != nil {
		return fmt.Errorf("comment on the pull request %q: %w", link, err)
	}

	return nil
}

// parseLink returns the repo and the number of the pull request at the link.
func parseLink(link string) (Repo, string, error) {
	match := linkPattern.FindStringSubmatch(link)
	if match == nil {
		return Repo{}, "", errors.New("want the link to a pull request on Bitbucket, such as " +
			"https://bitbucket.org/acme/demo/pull-requests/1")
	}

	repo := Repo{Workspace: match[1], Slug: match[2]}

	return repo, match[3], repo.check()
}

// post sends the fields, as JSON, to the address, as the user, and reads the JSON answer into the
// target, unless it's nil. Any answer of 2xx is a success.
func (c Client) post(ctx context.Context, address string, fields, target any) error {
	body, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("write the request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.Email, c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	answer := io.LimitReader(resp.Body, maxAnswer)

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return LoginError{refusal(resp.Status, answer)}
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		return refusal(resp.Status, answer)
	case target == nil:
		return nil
	}

	return json.UnmarshalRead(answer, target)
}

// refusal gives the error for an answer that isn't a success: its status, then the message and
// the detail of Bitbucket's error, if the answer holds one. A detail that isn't a string, such as
// the scopes that a token lacks, shows as its JSON.
func refusal(status string, answer io.Reader) error {
	var body struct {
		Error struct {
			Message string         `json:"message"`
			Detail  jsontext.Value `json:"detail"`
		} `json:"error"`
	}

	// An answer that isn't JSON, such as a proxy's page, leaves the status alone.
	_ = json.UnmarshalRead(answer, &body)

	detail := string(body.Error.Detail)

	var text string
	if json.Unmarshal(body.Error.Detail, &text) == nil {
		detail = text
	}

	parts := []string{status}

	for _, part := range []string{body.Error.Message, detail} {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}

	return errors.New(strings.Join(parts, ": "))
}

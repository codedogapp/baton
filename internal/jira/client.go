package jira

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"uuid"
)

// gateway is the address of Atlassian's API gateway for Jira, which takes a scoped API token, as a
// site's own address doesn't (fact 13).
const gateway = "https://api.atlassian.com/ex/jira/"

// fields names the fields of a ticket that baton shows, so that Jira leaves the others out.
const fields = "summary,issuetype,status,parent,description,subtasks,issuelinks"

// maxAnswer is the most of an answer that baton reads, so that a wrong answer can't fill its
// memory.
const maxAnswer = 16 << 20

// keyPattern matches a ticket's key, in any case: its project's key, a dash and its number. So a
// key can't change the address that it goes into.
var keyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-[0-9]+$`)

// Client reads tickets from a Jira Cloud site as the user, with their email and a scoped API token
// that reads Jira (decision 6).
type Client struct {
	HTTP  *http.Client
	Site  string // the site's host, such as example.atlassian.net
	Email string
	Token string
}

// Ticket reads the ticket with the key, which it takes in any case, as Jira's keys are in capitals,
// and all its comments.
func (c Client) Ticket(ctx context.Context, key string) (Ticket, error) {
	if !keyPattern.MatchString(key) {
		return Ticket{}, fmt.Errorf("want a ticket's key, such as DEMO-1, not %q", key)
	}

	key = strings.ToUpper(key)

	cloud, err := c.cloudID(ctx)
	if err != nil {
		return Ticket{}, err
	}

	issue := gateway + cloud + "/rest/api/3/issue/" + key

	var ticket Ticket
	if err := c.get(ctx, issue+"?fields="+fields, true, &ticket); err != nil {
		return Ticket{}, fmt.Errorf("read %s: %w", key, err)
	}

	if ticket.Comments, err = c.comments(ctx, issue+"/comment"); err != nil {
		return Ticket{}, fmt.Errorf("read the comments on %s: %w", key, err)
	}

	return ticket, nil
}

// cloudID reads the site's cloud ID, which the gateway's addresses hold, from the site's
// tenant_info, which takes no login, so that the token goes to the gateway alone (decision 59).
func (c Client) cloudID(ctx context.Context) (string, error) {
	var tenant struct {
		CloudID string `json:"cloudId"`
	}

	if err := c.get(ctx, "https://"+c.Site+"/_edge/tenant_info", false, &tenant); err != nil {
		return "", fmt.Errorf("read the cloud ID of %s: %w", c.Site, err)
	}

	id, err := uuid.Parse(tenant.CloudID)
	if err != nil {
		return "", fmt.Errorf("read the cloud ID of %s: %w", c.Site, err)
	}

	return id.String(), nil
}

// comments reads a ticket's comments, oldest first, a page at a time, until it has as many as Jira
// counts, or a page comes empty, as one can, since the count can change between pages (fact 40).
func (c Client) comments(ctx context.Context, address string) ([]Comment, error) {
	var comments []Comment

	for {
		var page struct {
			Comments []Comment `json:"comments"`
			Total    int       `json:"total"`
		}

		query := "?orderBy=created&startAt=" + strconv.Itoa(len(comments))
		if err := c.get(ctx, address+query, true, &page); err != nil {
			return nil, err
		}

		comments = append(comments, page.Comments...)
		if len(page.Comments) == 0 || len(comments) >= page.Total {
			return comments, nil
		}
	}
}

// get reads the JSON answer to a GET of the address into target. With login, it sends the user's
// email and token, which only the gateway gets.
func (c Client) get(ctx context.Context, address string, login bool, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")

	if login {
		req.SetBasicAuth(c.Email, c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	answer := io.LimitReader(resp.Body, maxAnswer)
	if resp.StatusCode != http.StatusOK {
		return refusal(resp.Status, answer)
	}

	return json.UnmarshalRead(answer, target)
}

// refusal gives the error for an answer other than 200 OK: its status, and the messages that it
// holds, Jira's in errorMessages, or the gateway's in message.
func refusal(status string, answer io.Reader) error {
	var body struct {
		Messages []string `json:"errorMessages"`
		Message  string   `json:"message"`
	}

	// An answer that isn't JSON, such as a proxy's page, leaves the status alone.
	_ = json.UnmarshalRead(answer, &body)

	text := strings.TrimSpace(strings.Join(body.Messages, " ") + " " + body.Message)
	if text == "" {
		return errors.New(status)
	}

	return fmt.Errorf("%s: %s", status, text)
}

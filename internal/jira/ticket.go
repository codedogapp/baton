// Package jira reads a Jira Cloud ticket, which the orchestrator plans from, and gives it as
// Markdown to read (decisions 6 and 58).
package jira

import (
	"cmp"
	"slices"
	"strings"

	"github.com/codedogapp/baton/internal/adf"
)

// Ticket is a ticket as Jira's REST API gives it, with the fields that baton shows, and its
// comments, oldest first.
type Ticket struct {
	Key      string    `json:"key"`
	Fields   Fields    `json:"fields"`
	Comments []Comment `json:"-"`
}

// Fields are the fields of a ticket that baton shows.
type Fields struct {
	Summary     string   `json:"summary"`
	Type        Named    `json:"issuetype"`
	Status      Named    `json:"status"`
	Parent      Issue    `json:"parent"`
	Description adf.Node `json:"description"`
	Subtasks    []Issue  `json:"subtasks"`
	Links       []Link   `json:"issuelinks"`
}

// Named is a type or a status, which a ticket shows by its name.
type Named struct {
	Name string `json:"name"`
}

// Issue is another ticket that a ticket names, such as its parent or a subtask.
type Issue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  Named  `json:"status"`
	} `json:"fields"`
}

// Link links a ticket with another, its inward or its outward ticket, and its type says how, from
// the ticket's side: such as "blocks" for its outward ticket, and "is blocked by" for its inward.
type Link struct {
	Type struct {
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	} `json:"type"`
	Inward  Issue `json:"inwardIssue"`
	Outward Issue `json:"outwardIssue"`
}

// Comment is a comment on a ticket: who wrote it, when, as Jira gives the time, and its text.
type Comment struct {
	Author struct {
		Name string `json:"displayName"`
	} `json:"author"`
	Created string   `json:"created"`
	Body    adf.Node `json:"body"`
}

// Markdown renders the ticket as Markdown to read: its key and summary, its type, status and
// parent, then its description, subtasks, links and comments, each under its heading. A part that
// the ticket lacks shows nothing, not even its heading.
func (t Ticket) Markdown() string {
	fields := t.Fields

	about := "- Type: " + fields.Type.Name + "\n- Status: " + fields.Status.Name
	if fields.Parent.Key != "" {
		about += "\n- Parent: " + fields.Parent.line()
	}

	comments := make([]string, len(t.Comments))
	for i, comment := range t.Comments {
		author := cmp.Or(comment.Author.Name, "Someone")
		comments[i] = join("### "+author+", "+comment.Created, adf.Markdown(comment.Body))
	}

	return join(
		"# "+t.Key+": "+fields.Summary,
		about,
		section("Description", adf.Markdown(fields.Description)),
		section("Subtasks", list(fields.Subtasks, Issue.line)),
		section("Links", list(fields.Links, Link.line)),
		section("Comments", join(comments...)),
	) + "\n"
}

// section gives a part of the ticket under its heading, or nothing for an empty part.
func section(heading, text string) string {
	if text == "" {
		return ""
	}

	return "## " + heading + "\n\n" + text
}

// join joins the parts that aren't empty, with a blank line between them.
func join(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(part string) bool { return part == "" }),
		"\n\n")
}

// list renders items as a bullet list, a line each.
func list[T any](items []T, line func(T) string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = "- " + line(item)
	}

	return strings.Join(lines, "\n")
}

// line shows a ticket that another names by its key, summary and status.
func (i Issue) line() string {
	return i.Key + ": " + i.Fields.Summary + " (" + i.Fields.Status.Name + ")"
}

// line shows a link from the ticket's side, by its type and the other ticket.
func (l Link) line() string {
	if l.Inward.Key != "" {
		return l.Type.Inward + " " + l.Inward.line()
	}

	return l.Type.Outward + " " + l.Outward.line()
}

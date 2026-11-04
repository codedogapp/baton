// Package adf renders the Atlassian Document Format, the JSON in which Jira's REST API gives a
// ticket's description and comments, as Markdown to read (decision 57).
package adf

import (
	"cmp"
	"slices"
	"strings"
)

// Node is a node of a document: a block, such as a paragraph or a table, or an inline node, such
// as a run of text or a mention.
type Node struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Marks   []Mark         `json:"marks"`
	Content []Node         `json:"content"`
}

// Mark styles a run of text, such as strong or a link.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

// Markdown renders a node, most often a whole document, as Markdown. The Markdown is for reading,
// so the text stays as Jira has it, and a node that text can't show, such as an image, leaves a
// word in its place. A node of an unknown type shows its content, or the text that stands in for
// it.
func Markdown(node Node) string {
	return blocks([]Node{node}, "\n\n")
}

// blocks renders nodes one after the other, with sep between them. A run of inline nodes makes
// one block, and an empty block makes none.
func blocks(nodes []Node, sep string) string {
	parts := make([]string, 0, len(nodes))
	for len(nodes) > 0 {
		if run := inlineRun(nodes); run > 0 {
			parts, nodes = append(parts, inline(nodes[:run])), nodes[run:]
		} else {
			parts, nodes = append(parts, block(nodes[0])), nodes[1:]
		}
	}

	return join(sep, parts...)
}

// join joins the parts that aren't empty, with sep between them.
func join(sep string, parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(part string) bool { return part == "" }), sep)
}

// inlineRun returns how many of the nodes, from the first on, are inline nodes.
func inlineRun(nodes []Node) int {
	end := slices.IndexFunc(nodes, func(node Node) bool { return !isInline(node.Type) })
	if end < 0 {
		return len(nodes)
	}

	return end
}

// isInline tells the nodes that flow in a line of text from the blocks.
func isInline(kind string) bool {
	return slices.Contains([]string{
		"text", "hardBreak", "mention", "emoji", "date", "status", "inlineCard",
		"inlineExtension", "placeholder", "mediaInline",
	}, kind)
}

// standIn returns the text that stands in for a node without content: a mention's name, an
// emoji, a status, a card's address or an extension's text.
func standIn(node Node) string {
	return cmp.Or(attr(node.Attrs, "text"), attr(node.Attrs, "shortName"), attr(node.Attrs, "url"))
}

// attr returns the text of the attribute, or "" if there is none.
func attr(attrs map[string]any, key string) string {
	text, _ := attrs[key].(string)

	return text
}

// number returns the number in the attribute, or fallback if there is none.
func number(attrs map[string]any, key string, fallback int) int {
	value, ok := attrs[key].(float64)
	if !ok {
		return fallback
	}

	return int(value)
}

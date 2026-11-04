package adf

import (
	"cmp"
	"strings"
)

// deepestHeading is the deepest level that a heading can have, in ADF and in Markdown.
const deepestHeading = 6

// block renders a block node.
func block(node Node) string {
	switch node.Type {
	case "paragraph":
		return inline(node.Content)
	case "heading":
		return heading(node)
	case "bulletList", "orderedList", "taskList", "decisionList":
		return list(node)
	case "codeBlock":
		return codeBlock(node)
	case "blockquote", "panel":
		return prefix(blocks(node.Content, "\n\n"), "> ", "> ")
	case "rule":
		return "---"
	case "table":
		return table(node)
	case "media":
		return attachment(node)
	case "expand", "nestedExpand":
		return expand(node)
	default:
		return cmp.Or(blocks(node.Content, "\n\n"), standIn(node))
	}
}

// prefix puts first before the first line of the text and rest before each later line, without
// the spaces at their end on an empty line.
func prefix(text, first, rest string) string {
	lines := strings.Split(text, "\n")
	lead := first

	for index, line := range lines {
		if line == "" {
			lead = strings.TrimRight(lead, " ")
		}

		lines[index], lead = lead+line, rest
	}

	return strings.Join(lines, "\n")
}

// heading renders a heading at its level.
func heading(node Node) string {
	level := min(max(number(node.Attrs, "level", 1), 1), deepestHeading)

	return strings.Repeat("#", level) + " " + inline(node.Content)
}

// codeBlock renders a code block in a fence that its code can't end.
func codeBlock(node Node) string {
	code := inline(node.Content)
	ticks := fence(code, len("```"))

	return ticks + attr(node.Attrs, "language") + "\n" + code + "\n" + ticks
}

// expand renders an expand, a section that Jira shows folded, under its title in bold.
func expand(node Node) string {
	title := attr(node.Attrs, "title")
	if title != "" {
		title = "**" + title + "**"
	}

	return join("\n\n", title, blocks(node.Content, "\n\n"))
}

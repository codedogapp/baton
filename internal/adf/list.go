package adf

import (
	"strconv"
	"strings"
)

// list renders a list: each item after its marker, with the item's later lines indented under
// its first.
func list(node Node) string {
	start := number(node.Attrs, "order", 1)
	items := make([]string, 0, len(node.Content))

	for i, item := range node.Content {
		marker, body := bullet(node.Type, item, start+i), blocks(item.Content, "\n")
		if item.Type == node.Type { // a task list holds a nested list as it is, not in an item
			marker, body = "  ", block(item)
		}

		items = append(items, prefix(body, marker, strings.Repeat(" ", len(marker))))
	}

	return strings.Join(items, "\n")
}

// bullet returns the marker of the list's item: its number in an ordered list, and its box in a
// task list.
func bullet(kind string, item Node, position int) string {
	switch {
	case kind == "orderedList":
		return strconv.Itoa(position) + ". "
	case attr(item.Attrs, "state") == "DONE":
		return "- [x] "
	case kind == "taskList":
		return "- [ ] "
	default:
		return "- "
	}
}

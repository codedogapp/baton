package adf

import "cmp"

// block renders a block node.
func block(node Node) string {
	switch node.Type {
	case "paragraph":
		return inline(node.Content)
	case "media":
		return attachment(node)
	default:
		return cmp.Or(blocks(node.Content), standIn(node))
	}
}

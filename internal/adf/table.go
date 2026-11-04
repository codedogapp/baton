package adf

import "strings"

// table renders a table as a Markdown table, every row as wide as the widest. Its first row is
// the header, since a Markdown table needs one.
func table(node Node) string {
	rows := make([][]string, 0, len(node.Content))
	width := 0

	for _, row := range node.Content {
		cells := make([]string, 0, len(row.Content))
		for _, cell := range row.Content {
			cells = append(cells, cellText(cell))
		}

		rows = append(rows, cells)
		width = max(width, len(cells))
	}

	lines := make([]string, 0, len(rows)+1)
	for i, cells := range rows {
		cells = append(cells, make([]string, width-len(cells))...)

		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if i == 0 {
			lines = append(lines, "|"+strings.Repeat(" --- |", width))
		}
	}

	return strings.Join(lines, "\n")
}

// cellText renders a cell's blocks on one line, as a Markdown table's cell holds no more: a line
// break becomes <br>, and a | is escaped.
func cellText(cell Node) string {
	return strings.NewReplacer("|", `\|`, "\n", "<br>").Replace(blocks(cell.Content, "\n"))
}

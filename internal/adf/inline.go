package adf

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// inline renders inline nodes as one run of text.
func inline(nodes []Node) string {
	var text strings.Builder
	for _, node := range nodes {
		text.WriteString(span(node))
	}

	return text.String()
}

// span renders an inline node.
func span(node Node) string {
	switch node.Type {
	case "text":
		return styled(node)
	case "hardBreak":
		return "\n"
	case "date":
		return date(attr(node.Attrs, "timestamp"))
	case "mediaInline":
		return attachment(node)
	default:
		return cmp.Or(inline(node.Content), standIn(node))
	}
}

// styled renders a run of text with its marks: code innermost, since code shows its text as it
// is, and a link outermost. Other marks, such as a colour, leave the text plain.
func styled(node Node) string {
	text := node.Text
	if _, ok := find(node.Marks, "code"); ok {
		text = code(text)
	}

	for _, style := range []struct{ mark, delim string }{{"em", "*"}, {"strong", "**"}, {"strike", "~~"}} {
		if _, ok := find(node.Marks, style.mark); ok {
			text = style.delim + text + style.delim
		}
	}

	if link, ok := find(node.Marks, "link"); ok {
		text = "[" + text + "](" + attr(link.Attrs, "href") + ")"
	}

	return text
}

// find returns the mark of the type, if the marks hold one.
func find(marks []Mark, kind string) (Mark, bool) {
	i := slices.IndexFunc(marks, func(mark Mark) bool { return mark.Type == kind })
	if i < 0 {
		return Mark{}, false
	}

	return marks[i], true
}

// code renders text as a code span. A backtick at either end of the text would join the fence, so
// a space stands between them, which Markdown drops.
func code(text string) string {
	pad := ""
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		pad = " "
	}

	ticks := fence(text, 1)

	return ticks + pad + text + pad + ticks
}

// fence returns the shortest run of backticks, at least least long, that the code doesn't hold,
// so that the code can't end its own fence.
func fence(code string, least int) string {
	ticks := strings.Repeat("`", least)
	for strings.Contains(code, ticks) {
		ticks += "`"
	}

	return ticks
}

// date renders a date's timestamp, milliseconds since the epoch, as its day in UTC, as Jira does.
// A timestamp that isn't a number shows as it is.
func date(timestamp string) string {
	millis, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return timestamp
	}

	return time.UnixMilli(millis).UTC().Format(time.DateOnly)
}

// attachment stands in for a file or an image, which text can't show, with its name, or its
// address if it has no name.
func attachment(node Node) string {
	name := cmp.Or(attr(node.Attrs, "alt"), attr(node.Attrs, "url"))
	if name == "" {
		return "[attachment]"
	}

	return "[attachment: " + name + "]"
}

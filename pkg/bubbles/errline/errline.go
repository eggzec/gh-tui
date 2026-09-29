// Package errline lays out an error the way the bubbles and the app show
// one: a mark and the text, wrapped between words and cut with an
// ellipsis, then a hint that is never cut, such as "r to retry".
package errline

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Styles are how Lines draws an error.
type Styles struct {
	// Mark is the glyph before the text, or "" for none. The text's other
	// lines are indented by its width.
	Mark string
	// Separator goes between the text and the hint, and Ellipsis ends the
	// text where it is cut.
	Separator, Ellipsis string
	Text, Hint          lipgloss.Style
}

// Lines renders an error in lines of at most width cells: the mark and the
// text, wrapped to at most rows lines and ending in the ellipsis when it
// needs more, then the separator and the hint. The hint goes after the
// text where it fits, else on a line of its own, unindented if the indent
// would make it wrap; it wraps only where the width can't hold it whole. Empty text leaves the hint
// alone. The text and the hint are plain.
func Lines(s Styles, text, hint string, width, rows int) []string {
	width, rows = max(width, 1), max(rows, 1)
	lead := ""
	if s.Mark != "" {
		lead = s.Mark + " "
	}
	indent := strings.Repeat(" ", ansi.StringWidth(lead))
	if width <= len(indent) {
		// Too narrow for the mark and a letter besides.
		lead, indent = "", ""
	}
	inner := width - len(indent)

	wrapped := Wrap(text, inner)
	if len(wrapped) > rows {
		cut := ansi.Truncate(wrapped[rows-1]+" "+wrapped[rows], max(inner-ansi.StringWidth(s.Ellipsis), 0), "")
		wrapped = append(wrapped[:rows-1], strings.TrimRight(cut, " ")+s.Ellipsis)
	}
	lines := make([]string, 0, len(wrapped)+1)
	for i, r := range wrapped {
		pre := indent
		if i == 0 {
			pre = lead
		}
		lines = append(lines, s.Text.Render(pre+r))
	}
	if hint == "" {
		return lines
	}
	tail := s.Separator + hint
	if n := len(wrapped); n > 0 && ansi.StringWidth(wrapped[n-1])+ansi.StringWidth(tail) <= inner {
		lines[n-1] += s.Hint.Render(tail)
		return lines
	}
	if w := ansi.StringWidth(hint); w > inner && w <= width {
		// The hint loses the indent rather than wrap.
		indent = ""
	}
	for _, r := range Wrap(hint, width-len(indent)) {
		lines = append(lines, indent+s.Hint.Render(r))
	}
	return lines
}

// Wrap wraps s, plain text, to rows of width cells. It breaks only
// between words, not at hyphens as ansi.Wrap does, so that a path stays
// whole, and cuts a word only when it alone is wider. A character wider
// than the row shows as "…".
func Wrap(s string, width int) []string {
	var rows []string
	row := ""
	for w := range strings.FieldsSeq(s) {
		for ansi.StringWidth(w) > width {
			if row != "" {
				rows, row = append(rows, row), ""
			}
			head := ansi.Truncate(w, width, "")
			rest := w[len(head):]
			if head == "" {
				_, n := utf8.DecodeRuneInString(w)
				head, rest = "…", w[n:]
			}
			rows, w = append(rows, head), rest
		}
		switch {
		case w == "":
		case row == "":
			row = w
		case ansi.StringWidth(row)+1+ansi.StringWidth(w) <= width:
			row += " " + w
		default:
			rows, row = append(rows, row), w
		}
	}
	if row != "" {
		rows = append(rows, row)
	}
	return rows
}

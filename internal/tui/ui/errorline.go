package ui

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// errorTextLines is how many lines ErrorLine gives the text of an error;
// the hint may take more.
const errorTextLines = 2

// ErrorStyles are how ErrorLine draws an error.
type ErrorStyles struct {
	// Mark is the glyph before the text, or "" for none. ErrorLine indents
	// the text's other lines by its width.
	Mark string
	// Separator goes between the text and the hint, and Ellipsis ends the
	// text where it is cut.
	Separator, Ellipsis string
	Text                lipgloss.Style
	Hint                lipgloss.Style
}

// Errors returns the styles of error lines, marked with the error glyph of
// ic and cut and joined to their hints as ic does.
func (t Theme) Errors(ic Icons) ErrorStyles {
	return ErrorStyles{Mark: ic.Error, Separator: ic.Separator, Ellipsis: ic.Ellipsis, Text: t.Error, Hint: t.Subtle}
}

// Empty returns the styles of an empty state that says why a pane has
// nothing to show, such as data the token may not read: muted, and
// unmarked, since nothing went wrong, but cut and joined to its hint as ic
// does.
func (t Theme) Empty(ic Icons) ErrorStyles {
	return ErrorStyles{Separator: ic.Separator, Ellipsis: ic.Ellipsis, Text: t.Muted, Hint: t.Subtle}
}

// ErrorLine renders an error as Say words it, in lines of at most width
// cells: the mark and the text, which wraps to two lines and ends in the
// ellipsis when it needs more, then the separator and the hint. The hint goes on a line of
// its own when it doesn't fit after the text, and is never cut; it wraps
// only where the width can't hold it at all. The text and the hint are
// plain.
func ErrorLine(s ErrorStyles, text, hint string, width int) []string {
	width = max(width, 1)
	lead := s.Mark + " "
	if s.Mark == "" {
		lead = ""
	}
	indent := strings.Repeat(" ", ansi.StringWidth(lead))
	if width <= len(indent) {
		// Too narrow for the mark and a letter besides.
		lead, indent = "", ""
	}
	inner := width - len(indent)

	rows := wrapRows(text, inner)
	if len(rows) > errorTextLines {
		cut := ansi.Truncate(rows[errorTextLines-1]+" "+rows[errorTextLines], max(inner-ansi.StringWidth(s.Ellipsis), 0), "")
		rows = append(rows[:errorTextLines-1], strings.TrimRight(cut, " ")+s.Ellipsis)
	}
	lines := make([]string, 0, len(rows)+1)
	for i, r := range rows {
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
	if n := len(rows); n > 0 && ansi.StringWidth(rows[n-1])+ansi.StringWidth(tail) <= inner {
		lines[n-1] += s.Hint.Render(tail)
		return lines
	}
	for _, r := range wrapRows(hint, inner) {
		lines = append(lines, indent+s.Hint.Render(r))
	}
	return lines
}

// wrapRows wraps s, plain text, to rows of width cells. It breaks only
// between words, not at hyphens as ansi.Wrap does, so a path such as the
// log's stays whole, and cuts a word only when it alone is wider.
func wrapRows(s string, width int) []string {
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
				// A character wider than the row can't show.
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

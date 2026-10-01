package ui

import "github.com/eggzec/gh-tui/pkg/bubbles/errline"

// errorTextLines is how many lines ErrorLine gives the text of an error;
// the hint may take more.
const errorTextLines = 2

// ErrorStyles are how ErrorLine draws an error.
type ErrorStyles = errline.Styles

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
// ellipsis when it needs more, then the separator and the hint. The hint
// goes on a line of its own when it doesn't fit after the text, indented
// under the text, or unindented when the indent would make it wrap, and
// is never cut; it wraps only where the width can't hold it at all. The
// text and the hint are plain.
func ErrorLine(s ErrorStyles, text, hint string, width int) []string {
	return errline.Lines(s, text, hint, width, errorTextLines)
}

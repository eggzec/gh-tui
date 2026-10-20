package ownerui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Paint is a style rendered once into the sequences around its text, so
// rows are styled by concatenation. It suits styles of one line without
// padding or borders.
type Paint struct {
	pre, post string
}

// NewPaint renders st once into a paint.
func NewPaint(st lipgloss.Style) Paint {
	pre, post, _ := strings.Cut(st.Render("x"), "x")
	return Paint{pre: pre, post: post}
}

// Render styles s, and leaves it empty when it is.
func (p Paint) Render(s string) string {
	if s == "" {
		return ""
	}
	return p.pre + s + p.post
}

// Write writes s, styled, to b.
func (p Paint) Write(b *strings.Builder, s string) {
	if s == "" {
		return
	}
	b.WriteString(p.pre)
	b.WriteString(s)
	b.WriteString(p.post)
}

// Bold is p with bold added to what it sets, so a paint of a color keeps
// it.
func (p Paint) Bold() Paint {
	rest, ok := strings.CutPrefix(p.pre, "\x1b[")
	if !ok {
		return NewPaint(lipgloss.NewStyle().Bold(true))
	}
	return Paint{pre: "\x1b[1;" + rest, post: p.post}
}

// The looks of a row: as it is, selected in a blurred pane, and selected in
// a focused one.
const (
	LookPlain = iota
	LookBlurred
	LookFocused
	NumLooks
)

// Look returns the look of a row.
func Look(selected, focused bool) int {
	switch {
	case selected && focused:
		return LookFocused
	case selected:
		return LookBlurred
	}
	return LookPlain
}

// RowPaints are the paints of the cells of a row in one look. Title paints
// the name or the title of the row, which takes the accent in a focused
// pane.
type RowPaints struct {
	Title, Text, Muted, Subtle, Accent Paint
}

// NewRowPaints returns the paints of a row in each of its looks.
func NewRowPaints(t ui.Theme) [NumLooks]RowPaints {
	var rows [NumLooks]RowPaints
	for look := range rows {
		selected := look != LookPlain
		cell := func(s lipgloss.Style) Paint { return NewPaint(t.Cell(s, selected)) }
		rows[look] = RowPaints{
			Title:  cell(t.RowTitle(selected, look == LookFocused)),
			Text:   cell(t.Text),
			Muted:  cell(t.Muted),
			Subtle: cell(t.Subtle),
			Accent: cell(t.Accent),
		}
	}
	return rows
}

// Styles are the paints of the profile, the cards and the table.
type Styles struct {
	Name, Login, Text, Muted, Subtle, Accent Paint
	// Rows are the paints of the cells of the rows of lists, by look.
	Rows [NumLooks]RowPaints
	// Cursor and Blurred mark the selected card, while its pane is focused
	// and while it isn't.
	Cursor, Blurred string
}

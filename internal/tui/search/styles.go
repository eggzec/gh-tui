package search

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// styles are the page's own, built once per theme.
type styles struct {
	edge, focusEdge   paint
	title, focusTitle paint

	name, login, text, muted, subtle paint
	accent, success, warning, fail   paint
	selected                         paint
	// match marks what a code search matched.
	match paint
	// states color the glyphs of the states of issues and pull requests.
	states [ui.NumStates]paint

	// cursor and blurred mark the selected row or card, while the pane is
	// focused and while it isn't.
	cursor, blurred string

	// look holds the cells of a result in each of its looks.
	look [numLooks]cells
}

// The looks of a result: as it is, selected in a blurred pane, and selected
// in a focused one.
const (
	lookPlain = iota
	lookBlurred
	lookFocused
	numLooks
)

// cells are the paints of the cells of a result in one look.
type cells struct {
	states               [ui.NumStates]paint
	title, muted, subtle paint
	text                 paint
}

func newCells(t ui.Theme, look int) cells {
	selected := look != lookPlain
	cell := func(s lipgloss.Style) lipgloss.Style { return t.Cell(s, selected) }
	c := cells{
		title:  newPaint(t.Cell(t.RowTitle(selected, look == lookFocused), selected)),
		muted:  newPaint(cell(t.Muted)),
		subtle: newPaint(cell(t.Subtle)),
		text:   newPaint(cell(t.Text)),
	}
	for i := range c.states {
		c.states[i] = newPaint(cell(t.State(ui.State(i))))
	}
	return c
}

// lookOf returns the look of a result, for the pane being focused or not.
func lookOf(selected, focused bool) int {
	switch {
	case selected && focused:
		return lookFocused
	case selected:
		return lookBlurred
	}
	return lookPlain
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	var states [ui.NumStates]paint
	for i := range states {
		states[i] = newPaint(t.State(ui.State(i)))
	}
	var look [numLooks]cells
	for i := range look {
		look[i] = newCells(t, i)
	}
	return styles{
		look:       look,
		states:     states,
		edge:       newPaint(border),
		focusEdge:  newPaint(t.Accent),
		title:      newPaint(t.Muted),
		focusTitle: newPaint(t.Accent.Bold(true)),
		name:       newPaint(t.Title),
		login:      newPaint(t.Muted),
		text:       newPaint(t.Text),
		muted:      newPaint(t.Muted),
		subtle:     newPaint(t.Subtle),
		accent:     newPaint(t.Accent),
		success:    newPaint(t.Success),
		warning:    newPaint(t.Warning),
		fail:       newPaint(t.Error),
		selected:   newPaint(t.Title),
		match:      newPaint(t.Accent.Bold(true)),
		cursor:     t.Accent.Render(ic.Cursor) + " ",
		blurred:    t.Subtle.Render(ic.Cursor) + " ",
	}
}

// paint is a style rendered once into the sequences around its text, so
// rows are styled by concatenation. It suits styles of one line without
// padding or borders, which these are.
type paint struct {
	pre, post string
}

func newPaint(st lipgloss.Style) paint {
	pre, post, _ := strings.Cut(st.Render("x"), "x")
	return paint{pre: pre, post: post}
}

func (p paint) render(s string) string {
	if s == "" {
		return ""
	}
	return p.pre + s + p.post
}

func (p paint) write(b *strings.Builder, s string) {
	if s == "" {
		return
	}
	b.WriteString(p.pre)
	b.WriteString(s)
	b.WriteString(p.post)
}

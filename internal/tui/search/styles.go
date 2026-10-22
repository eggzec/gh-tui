package search

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// styles are the page's own, built once per theme.
type styles struct {
	// frames draws the boxes and names the strips.
	frames panes.Styles

	name, login, text, muted, subtle paint
	accent, success, warning, fail   paint
	selected                         paint
	// states color the glyphs of the states of issues and pull requests.
	states [ui.NumStates]paint

	// cursor and blurred mark the selected row or card, while the pane is
	// focused and while it isn't.
	cursor, blurred string

	// look holds the cells of a result in each of its looks.
	look [ui.NumLooks]cells
}

// cells are the paints of the cells of a result in one look.
type cells struct {
	states               [ui.NumStates]paint
	title, muted, subtle paint
	text                 paint
}

func newCells(t ui.Theme, look int) cells {
	selected := look != ui.LookPlain
	cell := func(s lipgloss.Style) lipgloss.Style { return t.Cell(s, selected) }
	c := cells{
		title:  newPaint(t.Cell(t.RowTitle(selected, look == ui.LookFocused), selected)),
		muted:  newPaint(cell(t.Muted)),
		subtle: newPaint(cell(t.Subtle)),
		text:   newPaint(cell(t.Text)),
	}
	for i := range c.states {
		c.states[i] = newPaint(cell(t.State(ui.State(i))))
	}
	return c
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	var states [ui.NumStates]paint
	for i := range states {
		states[i] = newPaint(t.State(ui.State(i)))
	}
	var look [ui.NumLooks]cells
	for i := range look {
		look[i] = newCells(t, i)
	}
	frames := panes.DefaultStyles(t.Dark)
	frames.Border = ic.Border
	frames.Ellipsis = ic.Ellipsis
	return styles{
		look:     look,
		states:   states,
		frames:   frames,
		name:     newPaint(t.Title),
		login:    newPaint(t.Muted),
		text:     newPaint(t.Text),
		muted:    newPaint(t.Muted),
		subtle:   newPaint(t.Subtle),
		accent:   newPaint(t.Accent),
		success:  newPaint(t.Success),
		warning:  newPaint(t.Warning),
		fail:     newPaint(t.Error),
		selected: newPaint(t.Title),
		cursor:   t.Accent.Render(ic.Cursor) + " ",
		blurred:  t.Subtle.Render(ic.Cursor) + " ",
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

package dashboard

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// styles are the dashboard's own, built once per theme.
type styles struct {
	edge, focusEdge   paint
	title, focusTitle paint

	name, login, text, muted, subtle paint
	accent, success, warning         paint
	selected                         paint
	// states color the glyphs of the states of issues and pull requests,
	// and langs those of languages, by name and color, as they are met.
	states [ui.NumStates]paint
	langs  map[string]paint

	// cursor and blurred mark the selected row or card, while the pane is
	// focused and while it isn't.
	cursor, blurred string
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	var states [ui.NumStates]paint
	for i := range states {
		states[i] = newPaint(t.State(ui.State(i)))
	}
	return styles{
		states:     states,
		langs:      map[string]paint{},
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
		selected:   newPaint(t.Title),
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

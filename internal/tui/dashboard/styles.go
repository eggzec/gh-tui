package dashboard

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// styles are the dashboard's own, built once per theme.
type styles struct {
	edge, focusEdge   ownerui.Paint
	title, focusTitle ownerui.Paint

	name, login, text, muted, subtle ownerui.Paint
	accent, success, warning         ownerui.Paint
	selected                         ownerui.Paint
	// states color the glyphs of the states of issues and pull requests,
	// and langs those of languages, by name and color, as they are met.
	states [ui.NumStates]ownerui.Paint
	langs  map[string]ownerui.Paint

	// cursor and blurred mark the selected row or card, while the pane is
	// focused and while it isn't.
	cursor, blurred string

	// shared are the paints of the profile and the pinned cards.
	shared ownerui.Styles
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	var states [ui.NumStates]ownerui.Paint
	for i := range states {
		states[i] = ownerui.NewPaint(t.State(ui.State(i)))
	}
	st := styles{
		states:     states,
		langs:      map[string]ownerui.Paint{},
		edge:       ownerui.NewPaint(border),
		focusEdge:  ownerui.NewPaint(t.Accent),
		title:      ownerui.NewPaint(t.Muted),
		focusTitle: ownerui.NewPaint(t.Accent.Bold(true)),
		name:       ownerui.NewPaint(t.Title),
		login:      ownerui.NewPaint(t.Muted),
		text:       ownerui.NewPaint(t.Text),
		muted:      ownerui.NewPaint(t.Muted),
		subtle:     ownerui.NewPaint(t.Subtle),
		accent:     ownerui.NewPaint(t.Accent),
		success:    ownerui.NewPaint(t.Success),
		warning:    ownerui.NewPaint(t.Warning),
		selected:   ownerui.NewPaint(t.Title),
		cursor:     t.Accent.Render(ic.Cursor) + " ",
		blurred:    t.Subtle.Render(ic.Cursor) + " ",
	}
	st.shared = ownerui.Styles{
		Name: st.name, Login: st.login, Text: st.text, Muted: st.muted,
		Subtle: st.subtle, Accent: st.accent,
		Cursor: st.cursor, Blurred: st.blurred,
	}
	return st
}

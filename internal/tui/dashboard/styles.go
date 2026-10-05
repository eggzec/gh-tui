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

	success, warning ownerui.Paint
	// states color the glyphs of the states of issues and pull requests,
	// and langs those of languages, by name and color, as they are met.
	states [ui.NumStates]ownerui.Paint
	langs  map[string]ownerui.Paint

	// shared are the paints the dashboard shares with the profile, the
	// pinned cards and the repositories: its text, and the cursor and the
	// blurred cursor that mark the selected row or card while the pane is
	// focused and while it isn't.
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
		success:    ownerui.NewPaint(t.Success),
		warning:    ownerui.NewPaint(t.Warning),
		shared: ownerui.Styles{
			Name:     ownerui.NewPaint(t.Title),
			Login:    ownerui.NewPaint(t.Muted),
			Text:     ownerui.NewPaint(t.Text),
			Muted:    ownerui.NewPaint(t.Muted),
			Subtle:   ownerui.NewPaint(t.Subtle),
			Accent:   ownerui.NewPaint(t.Accent),
			Selected: ownerui.NewPaint(t.Title),
			Cursor:   t.Accent.Render(ic.Cursor) + " ",
			Blurred:  t.Subtle.Render(ic.Cursor) + " ",
		},
	}
	return st
}

package dashboard

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// styles are the dashboard's own, built once per theme.
type styles struct {
	// frames draw the panes: the border, the titles and the strip.
	frames panes.Styles
	// focusTitle is the current tab of a pane.
	focusTitle ui.Paint

	success, warning ui.Paint
	// states color the glyphs of the states of issues and pull requests,
	// and langs those of languages, by name and color, as they are met.
	states [ui.NumStates]ui.Paint
	langs  map[string]ui.Paint

	// shared are the paints the dashboard shares with the profile, the
	// pinned cards and the repositories: its text, and the cursor and the
	// blurred cursor that mark the selected row or card while the pane is
	// focused and while it isn't.
	shared ui.ListStyles
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	var states [ui.NumStates]ui.Paint
	for i := range states {
		states[i] = ui.NewPaint(t.State(ui.State(i)))
	}
	frames := panes.DefaultStyles(t.Dark)
	frames.Border = ic.Border
	frames.Ellipsis = ic.Ellipsis
	frames.Separator = ic.Separator
	frames.Edge = border
	frames.EdgeFocus = t.Accent
	frames.Title = t.Muted
	frames.TitleFocus = t.Accent.Bold(true)
	frames.StripCurrent = t.Accent.Bold(true)
	frames.StripOther = t.Subtle
	frames.StripSep = t.Subtle
	st := styles{
		states:     states,
		langs:      map[string]ui.Paint{},
		frames:     frames,
		focusTitle: ui.NewPaint(t.Accent.Bold(true)),
		success:    ui.NewPaint(t.Success),
		warning:    ui.NewPaint(t.Warning),
		shared: ui.ListStyles{
			Name:    ui.NewPaint(t.Title),
			Login:   ui.NewPaint(t.Muted),
			Text:    ui.NewPaint(t.Text),
			Muted:   ui.NewPaint(t.Muted),
			Subtle:  ui.NewPaint(t.Subtle),
			Accent:  ui.NewPaint(t.Accent),
			Rows:    ui.NewRowPaints(t),
			Cursor:  t.Accent.Render(ic.Cursor) + " ",
			Blurred: t.Subtle.Render(ic.Cursor) + " ",
		},
	}
	return st
}

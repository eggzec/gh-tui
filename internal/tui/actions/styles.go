package actions

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// cursorGlyph marks the row under the cursor, as the feed does.
const cursorGlyph = "▌"

// sepWidth is the width of the rule between two panes, with a space on
// each side.
const sepWidth = 3

// styles are the modal's own, built once per theme.
type styles struct {
	title, focusTitle lipgloss.Style
	crumb, lastCrumb  lipgloss.Style
	// sep is the rule between two panes, rendered.
	sep string
	// gutter is the rendered mark of the row under the cursor, in a focused
	// or a blurred pane, and noGutter its blank.
	gutter, blurGutter, noGutter string

	text, strong, muted, subtle lipgloss.Style
	accent, warning, error      lipgloss.Style
	// question styles the confirmation.
	question lipgloss.Style
	// states style the glyphs of the run states, and glyphs holds them
	// rendered.
	states [ui.NumRunStates]lipgloss.Style
	glyphs [ui.NumRunStates]string
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	s := styles{
		title:      t.Muted,
		focusTitle: t.Accent.Bold(true),
		crumb:      t.Muted,
		lastCrumb:  t.Accent.Bold(true),
		sep:        border.Render(" │ "),
		gutter:     t.Accent.Render(cursorGlyph) + " ",
		blurGutter: t.Subtle.Render(cursorGlyph) + " ",
		noGutter:   "  ",
		text:       t.Text,
		strong:     t.Title,
		muted:      t.Muted,
		subtle:     t.Subtle,
		accent:     t.Accent,
		warning:    t.Warning,
		error:      t.Error,
		question:   t.Accent.Bold(true),
	}
	for st := range ui.NumRunStates {
		s.states[st] = t.Run(st)
		s.glyphs[st] = s.states[st].Render(ic.Run(st))
	}
	return s
}

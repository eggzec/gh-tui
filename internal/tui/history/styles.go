package history

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// cursorGlyph marks the row under the cursor, as the graph does.
const cursorGlyph = "▌"

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
	accent, success, warning    lipgloss.Style
	error                       lipgloss.Style
	// label styles the names of the header's fields.
	label lipgloss.Style
	// baseMark is the rendered mark of the branch the files show.
	baseMark string
}

func newStyles(t ui.Theme) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	return styles{
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
		success:    t.Success,
		warning:    t.Warning,
		error:      t.Error,
		label:      t.Muted,
		baseMark:   t.Accent.Render("●"),
	}
}

// sepWidth is the width of the rule between two panes, with a space on
// each side.
const sepWidth = 3

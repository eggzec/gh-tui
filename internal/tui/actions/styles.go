package actions

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// cursorGlyph marks the row under the cursor, as the feed does.
// sepWidth is the width of the rule between two panes, with a space on
// each side.
const sepWidth = 3

// The looks of a run row: as it is, selected in a blurred pane, and
// selected in a focused one.
const (
	lookPlain = iota
	lookBlurred
	lookFocused
	numLooks
)

// styles are the modal's own, built once per theme.
type styles struct {
	// ic draws the crumbs, separators and ellipses.
	ic                ui.Icons
	title, focusTitle lipgloss.Style
	crumb, lastCrumb  lipgloss.Style
	// sep is the rule between two panes, rendered.
	sep string
	// gutter is the rendered mark of the row under the cursor, in a focused
	// or a blurred pane, and noGutter its blank.
	gutter, blurGutter, noGutter string
	// folded and unfolded mark a group of jobs, rendered.
	folded, unfolded string

	ui.RunStyles
	// row holds the styles of the cells of a run row in each of its looks.
	row [numLooks]ui.RunStyles
	// confirm styles the confirmation.
	confirm ui.ConfirmStyles
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	s := styles{
		ic:         ic,
		title:      t.Muted,
		focusTitle: t.Accent.Bold(true),
		crumb:      t.Muted,
		lastCrumb:  t.Accent.Bold(true),
		sep:        border.Render(" " + ic.Border.Left + " "),
		gutter:     t.Accent.Render(ic.Cursor) + " ",
		blurGutter: t.Subtle.Render(ic.Cursor) + " ",
		noGutter:   "  ",
		folded:     t.Muted.Render(ic.Folded) + " ",
		unfolded:   t.Muted.Render(ic.Unfolded) + " ",
		RunStyles:  ui.NewRunStyles(t, ic),
		confirm:    t.Confirm(ic),
	}
	s.row = [numLooks]ui.RunStyles{
		lookPlain:   ui.NewRowRunStyles(t, ic, false, false),
		lookBlurred: ui.NewRowRunStyles(t, ic, true, false),
		lookFocused: ui.NewRowRunStyles(t, ic, true, true),
	}
	return s
}

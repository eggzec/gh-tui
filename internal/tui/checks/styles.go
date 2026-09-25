package checks

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// styles are the step's own, built once per theme.
type styles struct {
	run              ui.RunStyles
	crumb, lastCrumb lipgloss.Style
	// group styles the titles of the groups, and required the marker of
	// the checks a merge needs.
	group, required lipgloss.Style
	question        lipgloss.Style
	// gutter is the rendered mark of the row under the cursor, and
	// noGutter its blank.
	gutter, noGutter string
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	return styles{
		run:       ui.NewRunStyles(t, ic),
		crumb:     t.Muted,
		lastCrumb: t.Accent.Bold(true),
		group:     t.Muted.Bold(true),
		required:  t.Warning,
		question:  t.Accent.Bold(true),
		gutter:    t.Accent.Render("▌") + " ",
		noGutter:  "  ",
	}
}

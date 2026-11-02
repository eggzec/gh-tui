package actions

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
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
	// tree draws the guides of the job rows, as the trees of the panes do, and
	// level is the cells of one level of them.
	tree  treeview.Styles
	level int

	ui.RunStyles
	// row holds the styles of the cells of a run row in each of its looks.
	row [ui.NumLooks]ui.RunStyles
	// confirm styles the confirmation.
	confirm confirm.Styles
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	tree := t.TreeView(ic)
	return styles{
		tree:       tree,
		level:      ansi.StringWidth(treeview.Prefix(tree, 1, true, nil)),
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
		row: [ui.NumLooks]ui.RunStyles{
			ui.LookPlain:   ui.NewRowRunStyles(t, ic, false, false),
			ui.LookBlurred: ui.NewRowRunStyles(t, ic, true, false),
			ui.LookFocused: ui.NewRowRunStyles(t, ic, true, true),
		},
	}
}

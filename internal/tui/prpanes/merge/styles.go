package merge

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// Styles are what the rows and lines are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change.
type Styles struct {
	// Checks draws the rows of the checks.
	Checks checktree.Styles
	// Ready is the glyph that marks a pull request with no blocker, in the
	// color of success.
	Ready string

	// Files draws the changed files that an owner owns.
	Files files.Styles

	tree     treeview.Styles
	warn, ok lipgloss.Style
	glyphs   ui.GlyphStyles
	dim      lipgloss.Style
	bold     lipgloss.Style
	accent   lipgloss.Style
	arrow    string
	sep      string
	// separator is the icon set's separator, unstyled, for text that is joined.
	separator string
	top       string
	ellipsis  string
}

// NewStyles returns the styles for theme t and the icon set named set, whose
// icons are ic.
func NewStyles(t ui.Theme, ic ui.Icons, set string) Styles {
	ck := checktree.NewStyles(t, ic, set)
	top := ic.Border.Top
	if top == "" {
		top = "-"
	}
	// The tree of an owner's files has no cursor, so its column stays blank.
	tree := t.TreeView(ic)
	tree.CursorGlyph = " "
	return Styles{
		Checks:    ck,
		Files:     files.NewStyles(t, ic, set),
		tree:      tree,
		warn:      t.Warning,
		ok:        t.Success,
		Ready:     t.Success.Render(ck.Glyphs.Pass),
		glyphs:    ui.NewGlyphStyles(t, ic),
		dim:       t.Subtle,
		bold:      lipgloss.NewStyle().Bold(true),
		accent:    t.Accent,
		arrow:     t.Subtle.Render(" " + ic.Arrow + " "),
		sep:       t.Subtle.Render(ic.Separator),
		top:       top,
		ellipsis:  ic.Ellipsis,
		separator: ic.Separator,
	}
}

// row returns the row of a blocker, which enter does something of its own on.
func (s *Styles) row(id string, g ui.Glyph, line string) treeview.Node {
	return treeview.Node{ID: id, Lines: []string{s.glyphs.Render(g) + " " + line}, Action: true}
}

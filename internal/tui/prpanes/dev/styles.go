package dev

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// Glyphs are the marks that the rows draw besides the state of an item.
type Glyphs struct {
	// Blocked marks what stands in the way of an issue.
	Blocked string
	// Glyphs are the marks the panes share: Separator goes between the parts
	// of a line, and Ellipsis ends a text that was cut.
	prpanes.Glyphs
	// Arrow leads from the head branch to the base, Minus goes before the
	// deleted lines, and Rule is drawn across to set the body apart.
	Arrow, Minus, Rule string
}

// newGlyphs returns the glyphs of the icon set ic.
func newGlyphs(ic ui.Icons) Glyphs {
	return Glyphs{
		Glyphs:  prpanes.GlyphsFrom(ic),
		Blocked: ic.Glyph(ui.GlyphBlocked),
		Arrow:   ic.Arrow, Minus: ic.Minus, Rule: ic.Border.Top,
	}
}

// Styles are what the rows are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change.
type Styles struct {
	Glyphs Glyphs
	// Tree styles the tree that holds the rows.
	Tree foldtree.Styles

	icons  ui.Icons
	states [ui.NumStates]lipgloss.Style
	bold   lipgloss.Style
	dim    lipgloss.Style
	warn   lipgloss.Style
	// chips draws the labels of a preview.
	chips ui.LabelChips
}

// NewStyles returns the styles for theme t and the icons ic.
func NewStyles(t ui.Theme, ic ui.Icons) Styles {
	s := Styles{
		Glyphs: newGlyphs(ic),
		Tree:   foldtree.DefaultStyles(t.Dark),
		icons:  ic,
		bold:   t.Text.Bold(true),
		dim:    t.Subtle,
		warn:   t.Warning,
		chips:  ui.NewLabelChips(t.Subtle.Padding(0, 1), t.Dark, ic.Ellipsis, labelNameWidth),
	}
	for i := range s.states {
		s.states[i] = t.State(ui.State(i))
	}
	return s
}

// labelNameWidth is the longest label name a chip of a preview shows.
const labelNameWidth = 24

// stateWords name the states, as a preview says them.
var stateWords = [ui.NumStates]string{
	ui.IssueOpen:       "open",
	ui.IssueClosed:     "closed",
	ui.IssueNotPlanned: "not planned",
	ui.PullOpen:        "open",
	ui.PullDraft:       "draft",
	ui.PullMerged:      "merged",
	ui.PullClosed:      "closed",
}

// stateWord says the state of r, or is empty for an item that couldn't be read.
func (s Styles) stateWord(r core.Reference) string {
	if r.Problem != "" {
		return ""
	}
	if r.Target.Kind == core.KindPull {
		return stateWords[ui.PullState(r.State, r.Draft)]
	}
	return stateWords[ui.IssueState(core.Issue{State: r.State, Reason: r.Reason})]
}

// ref draws the state of r in its colour, or a dim question mark for an item
// that couldn't be read.
func (s Styles) ref(r core.Reference) string {
	if r.Problem != "" {
		return s.dim.Render("?")
	}
	if r.Target.Kind == core.KindPull {
		return s.state(ui.PullState(r.State, r.Draft))
	}
	return s.state(ui.IssueState(core.Issue{State: r.State, Reason: r.Reason}))
}

// state draws the glyph of state st in its colour.
func (s Styles) state(st ui.State) string {
	return s.states[st].Render(s.icons.State(st))
}

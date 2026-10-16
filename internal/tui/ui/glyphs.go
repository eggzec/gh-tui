package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// Glyph names a glyph that pull request views draw: the verdicts of reviews,
// the state of review threads and checks, and the events of a timeline. The
// glyphs of states, runs and the rest are on [Icons].
type Glyph int

// Glyphs of pull request views.
const (
	// GlyphBlocked marks a pull request that cannot merge yet.
	GlyphBlocked Glyph = iota
	// GlyphApproved, GlyphChanges, GlyphCommented, GlyphRequested,
	// GlyphRerequested and GlyphDismissed mark a review: approved, asking
	// for changes, a comment only, one that was asked for, one asked for
	// again, and one that was dismissed.
	GlyphApproved
	GlyphChanges
	GlyphCommented
	GlyphRequested
	GlyphRerequested
	GlyphDismissed
	// GlyphUnresolved, GlyphResolved and GlyphOutdated mark a review
	// thread that is open, one that was resolved, and one whose code
	// changed since.
	GlyphUnresolved
	GlyphResolved
	GlyphOutdated
	// GlyphCheckPass, GlyphCheckFail and GlyphCheckPending mark the state
	// of a check.
	GlyphCheckPass
	GlyphCheckFail
	GlyphCheckPending
	// GlyphCommit, GlyphLabelEvent, GlyphReviewEvent and GlyphBranchEvent
	// mark an event of the timeline: a commit, a label change, a review and
	// a change of the branch.
	GlyphCommit
	GlyphLabelEvent
	GlyphReviewEvent
	GlyphBranchEvent
	// GlyphPlay marks what runs or starts, GlyphApps the app a check
	// belongs to, and GlyphAlert what needs attention.
	GlyphPlay
	GlyphApps
	GlyphAlert
	// NumGlyphs counts the glyphs, for arrays indexed by them.
	NumGlyphs
)

// glyphTable returns the glyphs of the icon set named set. None has padding:
// a row puts a space after a glyph.
func glyphTable(set string) [NumGlyphs]string {
	switch set {
	case config.IconsUnicode:
		return [NumGlyphs]string{
			GlyphBlocked: "⊘", GlyphApproved: "✓", GlyphChanges: "✗", GlyphCommented: "○",
			// A comment and a request would both be a ring, so the request
			// asks.
			GlyphRequested: "?", GlyphRerequested: "↻", GlyphDismissed: "⊘",
			GlyphUnresolved: "●", GlyphResolved: "✓", GlyphOutdated: "◌",
			GlyphCheckPass: "✓", GlyphCheckFail: "✗", GlyphCheckPending: "●",
			GlyphCommit: "•", GlyphLabelEvent: "#", GlyphReviewEvent: "◉", GlyphBranchEvent: "⎇",
			GlyphPlay: "▶", GlyphApps: "▣", GlyphAlert: "⚠",
		}
	case config.IconsASCII:
		return [NumGlyphs]string{
			GlyphBlocked: "!", GlyphApproved: "+", GlyphChanges: "x", GlyphCommented: "o",
			GlyphRequested: "?", GlyphRerequested: "~", GlyphDismissed: "-",
			GlyphUnresolved: "*", GlyphResolved: "+", GlyphOutdated: "~",
			GlyphCheckPass: "+", GlyphCheckFail: "x", GlyphCheckPending: "*",
			GlyphCommit: "*", GlyphLabelEvent: "#", GlyphReviewEvent: "@", GlyphBranchEvent: "Y",
			GlyphPlay: ">", GlyphApps: "#", GlyphAlert: "!",
		}
	default:
		// Octicons, as GitHub draws these.
		return [NumGlyphs]string{
			GlyphBlocked: "", GlyphApproved: "", GlyphChanges: "", GlyphCommented: "",
			GlyphRequested: "", GlyphRerequested: "", GlyphDismissed: "",
			GlyphUnresolved: "", GlyphResolved: "", GlyphOutdated: "",
			GlyphCheckPass: "", GlyphCheckFail: "", GlyphCheckPending: "",
			GlyphCommit: "", GlyphLabelEvent: "", GlyphReviewEvent: "", GlyphBranchEvent: "",
			GlyphPlay: "▶", GlyphApps: "▣", GlyphAlert: "",
		}
	}
}

// Glyph returns glyph g, or "" for a value that is none.
func (ic Icons) Glyph(g Glyph) string {
	if g < 0 || g >= NumGlyphs {
		return ""
	}
	return ic.glyphs[g]
}

// The colors of glyphs, on light and on dark backgrounds, after GitHub's:
// green, red, amber and purple, and the green of a resolved thread, which is
// quieter.
var (
	glyphGreen    = [2]string{"#1a7f37", "#3fb950"}
	glyphRed      = [2]string{"#cf222e", "#f85149"}
	glyphAmber    = [2]string{"#9a6700", "#d29922"}
	glyphResolved = [2]string{"#4b6b4b", "#5c7a5c"}
)

// glyphColors holds the color of each glyph that has one; the others are
// dim.
var glyphColors = [NumGlyphs]*[2]string{
	GlyphBlocked: &glyphRed, GlyphApproved: &glyphGreen, GlyphChanges: &glyphRed,
	GlyphRequested: &glyphAmber, GlyphRerequested: &glyphAmber,
	GlyphResolved:  &glyphResolved,
	GlyphCheckPass: &glyphGreen, GlyphCheckFail: &glyphRed, GlyphCheckPending: &glyphAmber,
	GlyphAlert: &glyphAmber,
}

// GlyphStyles draw the glyphs of pull request views in their colors. Build
// them once per theme with [NewGlyphStyles].
type GlyphStyles struct {
	glyphs [NumGlyphs]string
}

// NewGlyphStyles returns the glyphs of ic rendered in the colors of t.
func NewGlyphStyles(t Theme, ic Icons) GlyphStyles {
	var s GlyphStyles
	for g := range NumGlyphs {
		st := t.Muted
		if c := glyphColors[g]; c != nil {
			st = lipgloss.NewStyle().Foreground(lipgloss.Color(c[boolIndex(t.Dark)]))
		}
		s.glyphs[g] = st.Render(ic.Glyph(g))
	}
	return s
}

// Render returns glyph g in its color, or "" for a value that is none.
func (s GlyphStyles) Render(g Glyph) string {
	if g < 0 || g >= NumGlyphs {
		return ""
	}
	return s.glyphs[g]
}

func boolIndex(dark bool) int {
	if dark {
		return 1
	}
	return 0
}

package conv

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// maxName is the most cells the name of a person takes in a row, so that the
// text of a message keeps its room.
const maxName = 24

// Styles are what the rows are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change.
type Styles struct {
	// Glyphs are the marks that the icon sets of [ui] don't hold.
	Glyphs Glyphs

	glyphs ui.GlyphStyles
	icons  ui.Icons
	bold   lipgloss.Style
	dim    lipgloss.Style
	warn   lipgloss.Style
	// sha draws the short SHA of a commit.
	sha lipgloss.Style
	// chips draws the labels of the events, and keeps what it drew.
	chips *ui.LabelChips
	errs  ui.ErrorStyles
	// open is the amber glyph of a thread that isn't resolved.
	open string
	// snip are the styles of the code under a thread.
	snip snippetStyles
	// checks draws the rows of the checks under a commit.
	checks checktree.Styles
}

// NewStyles returns the styles for theme t and the icon set named set, whose
// icons are ic.
func NewStyles(t ui.Theme, ic ui.Icons, set string) Styles {
	chips := ui.NewLabelChips(t.Muted, t.Dark, ic.Ellipsis, maxName)
	return Styles{
		Glyphs: newGlyphs(set, ic),
		glyphs: ui.NewGlyphStyles(t, ic),
		icons:  ic,
		bold:   t.Text.Bold(true),
		dim:    t.Subtle,
		warn:   t.Warning,
		sha:    t.Subtle.Bold(true),
		chips:  &chips,
		errs:   t.Errors(ic),
		open:   t.Warning.Render(ic.Glyph(ui.GlyphUnresolved)),
		snip:   newSnippetStyles(t, ic, set),
		checks: checktree.NewStyles(t, ic, set),
	}
}

// review returns the glyph of a review in state st, or "" for one that has
// none.
func (s *Styles) review(st core.ReviewState) string {
	switch st {
	case core.ReviewStateApproved:
		return s.glyphs.Render(ui.GlyphApproved)
	case core.ReviewStateChangesRequested:
		return s.glyphs.Render(ui.GlyphChanges)
	case core.ReviewStateCommented:
		return s.glyphs.Render(ui.GlyphCommented)
	case core.ReviewStateDismissed:
		return s.glyphs.Render(ui.GlyphDismissed)
	case core.ReviewStatePending:
		return s.dim.Render(s.icons.Glyph(ui.GlyphCheckPending))
	}
	return ""
}

// thread returns the glyph of a thread that is resolved or not.
func (s *Styles) thread(resolved bool) string {
	if resolved {
		return s.glyphs.Render(ui.GlyphResolved)
	}
	return s.open
}

// outdated returns the glyph of a thread whose code changed since.
func (s *Styles) outdated() string { return s.glyphs.Render(ui.GlyphOutdated) }

// who returns the name of u, bold, with a dim tag for a bot: dim as a
// whole for a deleted account, or when quiet is set.
func (s *Styles) who(u core.User, quiet bool) string {
	name, gone := label(u)
	name = termtext.Truncate(name, maxName, s.Glyphs.Cut)
	if gone || quiet {
		name = s.dim.Render(name)
	} else {
		name = s.bold.Render(name)
	}
	if u.Bot {
		name += s.dim.Render(" bot")
	}
	return name
}

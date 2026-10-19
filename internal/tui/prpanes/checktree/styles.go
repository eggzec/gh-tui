package checktree

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Styles are what the rows are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change. The links they
// keep make each address and text once, so rows drawn on every frame cost
// no more: Styles are for the one goroutine that updates and draws.
type Styles struct {
	// Glyphs are the marks of the rows.
	Glyphs Glyphs
	// Errors draws the lines of an error.
	Errors ui.ErrorStyles

	links *termtext.Links
	dim   lipgloss.Style
	bold  lipgloss.Style
	// state holds the glyph of each check state, colored, and "" for none, and
	// tone the style that draws its word.
	state [core.CheckFailing + 1]string
	tone  [core.CheckFailing + 1]lipgloss.Style
	// actions, app and status are the dim glyphs of the kinds of checks.
	actions, app, status string
	// failure, warning and notice are the glyphs of the levels of annotations.
	failure, warning, notice string
}

// NewStyles returns the styles for theme t and the icon set named set, whose
// icons are ic.
func NewStyles(t ui.Theme, ic ui.Icons, set string) Styles {
	g := NewGlyphs(set, ic)
	dim := t.Subtle
	fail, pend, pass := t.Run(ui.RunFailure), t.Run(ui.RunInProgress), t.Run(ui.RunSuccess)
	s := Styles{
		Glyphs:  g,
		Errors:  t.Errors(ic),
		links:   new(termtext.Links),
		dim:     dim,
		bold:    lipgloss.NewStyle().Bold(true),
		actions: dim.Render(g.Actions),
		app:     dim.Render(g.App),
		status:  dim.Render(g.Status),
		failure: fail.Render(g.Failure),
		warning: pend.Render(g.Warning),
		notice:  dim.Render(g.Notice),
	}
	s.state[core.CheckFailing] = fail.Render(g.Fail)
	s.state[core.CheckPending] = pend.Render(g.Pending)
	s.state[core.CheckPassing] = pass.Render(g.Pass)
	s.tone[core.CheckFailing], s.tone[core.CheckPending], s.tone[core.CheckPassing] = fail, pend, pass
	return s
}

// icon returns the glyph that marks a check in state s.
func (s *Styles) icon(c core.CheckState) string {
	if c < 0 || int(c) >= len(s.state) {
		return ""
	}
	return s.state[c]
}

// Level returns the colored glyph that marks an annotation of level l: a level
// that GitHub adds later counts as a notice.
func (s *Styles) Level(l core.AnnotationLevel) string {
	switch l {
	case core.AnnotationFailure:
		return s.failure
	case core.AnnotationWarning:
		return s.warning
	case core.AnnotationNotice:
	}
	return s.notice
}

// link returns text, cleaned, as a link to addr when it is a safe address.
func (s *Styles) link(addr, text string) string {
	text = termtext.OneLine(text)
	if s.links == nil {
		return termtext.Link(addr, text)
	}
	return s.links.Link(addr, text)
}

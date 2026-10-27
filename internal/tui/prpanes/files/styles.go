package files

import (
	"path"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Styles are what the rows are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change.
type Styles struct {
	// Glyphs are the marks of the rows.
	Glyphs Glyphs

	bold, dim lipgloss.Style
	// accent is the style of the glyph and the SHA of a commit in the range.
	accent lipgloss.Style
	// add and del draw the lines a file adds and deletes.
	add, del lipgloss.Style
	// check ends the row of a file marked viewed, and rerequested leads the
	// note of one that changed since.
	check, rerequested string
	// failure, warning and notice are the glyphs of the levels of annotations,
	// colored as the checks tree draws them.
	failure, warning, notice string
	// fileIcon returns the icon of the file at a path, followed by a space, or
	// "" in an icon set without file icons.
	fileIcon func(path string) string
	// in and out are the commit glyph of a commit in the range and outside it.
	in, out string
	// state holds the glyph of each state of checks, colored, and "" for none.
	state [core.CheckFailing + 1]string
}

// NewStyles returns the styles for theme t and the icon set named set, whose
// icons are ic.
func NewStyles(t ui.Theme, ic ui.Icons, set string) Styles {
	g := newGlyphs(set, ic)
	d := t.Diff(ic)
	s := Styles{
		Glyphs:      g,
		bold:        t.Text.Bold(true),
		dim:         t.Subtle,
		accent:      t.Accent.Bold(true),
		add:         lipgloss.NewStyle().Foreground(d.Added.GetForeground()),
		del:         lipgloss.NewStyle().Foreground(d.Deleted.GetForeground()),
		check:       t.Subtle.Render(ic.Yes),
		rerequested: ui.NewGlyphStyles(t, ic).Render(ui.GlyphRerequested),
		failure:     t.Run(ui.RunFailure).Render(g.Failure),
		warning:     t.Run(ui.RunInProgress).Render(g.Warning),
		notice:      t.Subtle.Render(g.Notice),
		in:          t.Accent.Render(g.Commit),
		out:         t.Subtle.Render(g.Commit),
		fileIcon: func(p string) string {
			fi := ic.Entry(core.TreeEntry{Path: p, Name: path.Base(p)}, false)
			if fi.Glyph == "" {
				return ""
			}
			return t.FileIcon(fi).Render(fi.Glyph) + " "
		},
	}
	s.state[core.CheckFailing] = t.Run(ui.RunFailure).Render(g.Fail)
	s.state[core.CheckPending] = t.Run(ui.RunInProgress).Render(g.Pending)
	s.state[core.CheckPassing] = t.Run(ui.RunSuccess).Render(g.Pass)
	return s
}

// icon returns the glyph that marks checks in state c, or "" for none.
func (s *Styles) icon(c core.CheckState) string {
	if c < 0 || int(c) >= len(s.state) {
		return ""
	}
	return s.state[c]
}

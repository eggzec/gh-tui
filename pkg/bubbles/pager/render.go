package pager

import (
	"context"
)

// Render returns content rendered to fit width cells, as lines with
// colors of their own, such as markdown rendered for the terminal.
type Render func(width int) string

// SetRendered shows what render makes of source, named name, at the width
// of the text column, and renders it again whenever that width changes,
// keeping the place in it, the search and the filter. The rendered text
// is cleaned as any text with colors of its own is, and isn't
// highlighted. The line numbers, the status line, the search and the
// filter count and match the lines rendered, and the editor gets source,
// from its first line, since a rendered line has none of its own there.
func (m *Model) SetRendered(name, source string, render Render) {
	m.reset(name, stateReady, nil)
	m.raw, m.render = source, render
	m.renderedAt = -1
	m.fitRendered(false)
}

// Rendered reports whether the pager shows rendered content, as
// SetRendered gives it.
func (m Model) Rendered() bool { return m.render != nil }

// Rerender renders the content set with SetRendered again, at the same
// width, keeping the place, the search and the filter, for when what it
// renders to changed, such as when an image it shows arrived. It does
// nothing to other content.
func (m *Model) Rerender() {
	m.fitRendered(true)
}

// fitRendered renders the content set with SetRendered at the width of
// the text column, if that changed since the last render or force is
// set. The line at the top of the window stays as far into the content
// as it was. The filter and the search are run again over the new lines
// at once, whatever their size, since a parent that resizes the pager has
// no way to take a command.
func (m *Model) fitRendered(force bool) {
	w := m.textWidth()
	if m.render == nil || m.state != stateReady || w <= 0 || w == m.renderedAt && !force {
		return
	}
	top, n := m.topLine(), len(m.lines)
	s, p := m.search, m.proj
	// The gutter widens with the number of lines, which leaves less room
	// for the text, so the text renders again until it fits; a narrower
	// render never has fewer lines, so twice more is enough. The search
	// stays until then, since an inverted one has a gutter of its own.
	for range 3 {
		m.renderedAt = w
		m.setLines(m.render(w))
		if w = m.textWidth(); w == m.renderedAt || w <= 0 {
			break
		}
	}
	m.clearSearch()
	m.stopProjecting()
	m.gen++
	m.spans, m.vis, m.kept, m.mark = nil, nil, 0, -1
	m.top, m.row, m.left = 0, 0, 0
	if n > 0 {
		m.top = top * len(m.lines) / n
	}
	m.proj, m.want = projection{squeeze: p.squeeze}, projection{squeeze: p.squeeze}
	if !p.none() {
		vis, kept, _ := pick(context.Background(), m.lines, p)
		if p.filter.re == nil || kept > 0 {
			m.proj, m.want, m.vis, m.kept = p, p, vis, kept
			m.top = m.posOf(m.top)
		}
	}
	m.clamp()
	if s.re != nil {
		m.search = search{query: s.query, re: s.re, invert: s.invert, from: m.topLine(), cur: -1, stay: true}
		m.search.top, m.search.row = m.top, m.row
		lines, ends, _ := find(context.Background(), s.re, s.invert, m.lines, m.vis)
		m.found(lines, ends)
		if s.cur >= 0 && m.search.total() > 0 {
			// The current match goes to the first as far into the new
			// lines as the old one was.
			m.jump(m.firstFrom(s.curLine * len(m.lines) / max(n, 1)))
		}
	}
	m.enableSearchKeys()
}

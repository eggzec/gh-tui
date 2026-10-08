package pager

import (
	"context"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
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
//
// A rendered line that holds the kitty graphics protocol's Unicode
// placeholder draws an image: it reaches the terminal as it is, at the
// left edge of the text, and searches and filters see it as blank. Render
// must have made the rest of such a line safe to draw; every other line
// is cleaned, so no placeholder there draws an image.
func (m *Model) SetRendered(name, source string, render Render) {
	m.reset(name, stateReady, nil)
	m.raw, m.render = source, render
	m.renderedAt, m.owed = -1, false
	m.fitRendered(false)
}

// SetReserve sets what returns how many lines the rendered content may
// gain once it is shown, such as the rows of images still to arrive. The
// gutter is wide enough to number them, so that their arrival doesn't
// widen it and narrow the text, which renders the content again at
// another width.
func (m *Model) SetReserve(extra func() int) { m.reserve = extra }

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
		m.owed = m.owed && !force && w != m.renderedAt
		return
	}
	if m.owed && !force {
		return
	}
	m.owed = false
	top, n := m.topLine(), len(m.lines)
	// A resize says in atEnd whether the end was in view at the old size.
	end := n > 0 && (m.atEnd || force && m.atEndByChoice())
	m.atEnd = false
	s, p := m.search, m.proj
	at := m.anchorOf(top)
	curOff, wasShown := 0, false
	if s.cur >= 0 && s.curLine < n {
		curOff = m.offsetOf(s.curLine)
		wasShown = m.inView(m.posOf(s.curLine), 0)
	}
	oldTotal := s.total()
	// The gutter widens with the number of lines, which leaves less room
	// for the text. The first render guesses them from the lines of
	// before, or of the source, so that it seldom renders again, which
	// would ask for its images at another width too; a narrower render
	// never has fewer lines, so twice more is enough. The search stays
	// until then, since an inverted one has a gutter of its own.
	guess := n
	if guess == 0 {
		guess = strings.Count(m.raw, "\n") + 1
	}
	w = max(m.width-m.gutterFor(guess+m.reserved()), 1)
	for range 3 {
		m.renderedAt = w
		m.setLines(m.takePictures(m.render(w)))
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
		line := m.lineAt(at)
		m.top = line
		m.anchor = anchor{line: line, off: at, set: true}
	}
	m.proj, m.want = projection{squeeze: p.squeeze}, projection{squeeze: p.squeeze}
	if !p.none() {
		vis, kept, _ := pick(context.Background(), m.lines, p, m.pics)
		if p.filter.re == nil || kept > 0 {
			m.proj, m.want, m.vis, m.kept = p, p, vis, kept
			m.top = m.posOf(m.top)
		}
	}
	before := m.topLine()
	m.clamp()
	if end {
		// The reader was at the end, and stays there.
		m.top, m.row = m.last()
	}
	if n > 0 {
		// A clamp may have moved the top; the place the reader had is
		// kept for the next render all the same.
		m.anchor = anchor{line: m.topLine(), off: at, set: true, clamped: !end && m.topLine() != before}
	}
	if s.re != nil {
		m.search = search{query: s.query, re: s.re, invert: s.invert, from: m.topLine(), cur: -1, stay: true}
		m.search.top, m.search.row = m.top, m.row
		lines, ends, _ := find(context.Background(), s.re, s.invert, m.lines, m.vis)
		m.found(lines, ends)
		if s.cur >= 0 && m.search.total() > 0 {
			// The current match stays the same match, by its order, unless
			// the new lines hold another number of them, when it goes to
			// the first as far into the text as the old one was. The
			// window stays where it is unless the match was in view.
			top, row, left := m.top, m.row, m.left
			cur := s.cur
			if m.search.total() != oldTotal {
				cur = m.firstFrom(m.lineAt(curOff))
			}
			m.jump(cur)
			if !wasShown {
				m.top, m.row, m.left = top, row, left
				m.clamp()
			}
		}
	}
	m.enableSearchKeys()
}

// Settle starts the rest that rendered content waits after a resize,
// set with [WithResizeRest], and returns the command that ends it, or nil
// when nothing waits. Each call replaces the rest before it, so a resize
// that goes on renders once, at the last width, when the rest ends: the
// pager takes the message the command sends through Update.
func (m *Model) Settle() tea.Cmd {
	if !m.owed {
		return nil
	}
	m.sizeSeq++
	msg := settledMsg{id: m.id, seq: m.sizeSeq}
	return tea.Tick(m.resizeRest, func(time.Time) tea.Msg { return msg })
}

// settledMsg ends the rest of the pager with the ID that Settle began.
type settledMsg struct {
	id  int64
	seq int
}

// takePictures keeps the lines of text that draw images, as they are, by
// their index, and returns text with each of them blank.
func (m *Model) takePictures(text string) string {
	m.pics = nil
	if !strings.ContainsRune(text, termtext.Placeholder) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.ContainsRune(l, termtext.Placeholder) {
			if m.pics == nil {
				m.pics = make(map[int]string)
			}
			m.pics[i], lines[i] = l, ""
		}
	}
	return strings.Join(lines, "\n")
}

// writePicture writes l, a line that draws an image, cut to tw cells, the
// width of the text column, and pads it to them.
func writePicture(b *strings.Builder, l string, tw int) {
	if ansi.StringWidth(l) > tw {
		l = ansi.Truncate(l, tw, "")
	}
	b.WriteString(l)
	b.WriteString(ansi.ResetStyle)
	b.WriteString(strings.Repeat(" ", tw-ansi.StringWidth(l)))
}

// anchor is the place in rendered content that the window keeps across
// renders: off is how far into the text, in letters and digits, the line
// that was at the top is, and line is that line, so that the place is
// known again while the window stays there, and is not moved by the
// rounding of each render. Without it, a render at another width and back
// would drift off the place it began.
type anchor struct {
	line, off int
	set       bool
	// clamped is set when the end of the content moved the line there
	// from the place that off keeps.
	clamped bool
}

// weigh returns how much of the text a line holds: its letters and
// digits, and one for a line without any, so that every line has a place
// of its own.
func weigh(l string) int {
	n := 0
	for _, r := range l {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			n++
		}
	}
	return max(n, 1)
}

// viewsEnd reports whether the end of the content is in view.
func (m Model) viewsEnd() bool {
	if len(m.lines) == 0 {
		return false
	}
	top, row := m.last()
	return m.top > top || m.top == top && m.row >= row
}

// atEndByChoice reports whether the reader put the end of the content in
// view. A window that only a clamp put there isn't the reader's: the
// place the anchor keeps is where it goes back to.
func (m Model) atEndByChoice() bool {
	if !m.viewsEnd() {
		return false
	}
	return !m.anchor.set || !m.anchor.clamped || m.anchor.line != m.topLine()
}

// offsetOf returns how many letters and digits of the text come before
// line i.
func (m Model) offsetOf(i int) int {
	off := 0
	for _, l := range m.lines[:min(i, len(m.lines))] {
		off += weigh(l)
	}
	return off
}

// anchorOf returns how far into the text the line at the top, line, is:
// where the window kept it last if it still is at that line, and else
// the start of the line.
func (m Model) anchorOf(line int) int {
	if m.anchor.set && m.anchor.line == line {
		return m.anchor.off
	}
	return m.offsetOf(line)
}

// lineAt returns the index of the line that holds the place off letters
// and digits into the text, or the last line.
func (m Model) lineAt(off int) int {
	for i, l := range m.lines {
		w := weigh(l)
		if off < w {
			return i
		}
		off -= w
	}
	return max(len(m.lines)-1, 0)
}

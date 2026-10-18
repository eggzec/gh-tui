package appcheck

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// Step is a move of the cursor, or of the text when there are no annotations.
type Step int

// The moves.
const (
	Up Step = iota
	Down
	PageUp
	PageDown
	HalfPageUp
	HalfPageDown
	Top
	Bottom
)

// Model is the page of a check run in a window, with a cursor over its
// annotations.
type Model struct {
	data   Data
	opt    Options
	w, h   int
	page   Page
	built  bool
	cursor int
	top    int
}

// New returns a model that draws with o.
func New(o Options) Model { return Model{opt: o} }

// SetData sets what the page shows, and keeps the cursor on the annotation it
// was on when that is still there.
func (m *Model) SetData(d Data) {
	m.data = d
	m.built = false
	m.build()
}

// SetSize sets the window.
func (m *Model) SetSize(w, h int) {
	if w != m.w {
		m.built = false
	}
	m.w, m.h = w, h
	m.build()
}

// SetOptions sets what the page is drawn with, as a new theme asks.
func (m *Model) SetOptions(o Options) {
	m.opt = o
	m.built = false
	m.build()
}

func (m *Model) build() {
	if m.built || m.w <= 0 {
		return
	}
	m.page = Build(m.data, m.opt, m.w-1)
	m.built = true
	m.cursor = min(m.cursor, max(len(m.page.Rows)-1, 0))
	m.clamp()
}

// Page returns the text of the page for the width of the window.
func (m *Model) Page() Page {
	m.build()
	return m.page
}

// Selected returns the annotation under the cursor.
func (m *Model) Selected() (core.Annotation, bool) {
	m.build()
	if m.cursor < 0 || m.cursor >= len(m.page.Notes) {
		return core.Annotation{}, false
	}
	return m.page.Notes[m.cursor], true
}

// Move takes a step.
func (m *Model) Move(s Step) {
	m.build()
	if len(m.page.Rows) == 0 {
		m.scroll(s)
		return
	}
	last := len(m.page.Rows) - 1
	switch s {
	case Up:
		m.cursor--
	case Down:
		m.cursor++
	case PageUp:
		m.cursor -= max(m.h, 1)
	case PageDown:
		m.cursor += max(m.h, 1)
	case HalfPageUp:
		m.cursor -= max(m.h/2, 1)
	case HalfPageDown:
		m.cursor += max(m.h/2, 1)
	case Top:
		m.cursor, m.top = 0, 0
	case Bottom:
		m.cursor = last
	}
	m.cursor = min(max(m.cursor, 0), last)
	m.follow()
}

// scroll moves the text, for a page that has no annotations.
func (m *Model) scroll(s Step) {
	switch s {
	case Up:
		m.top--
	case Down:
		m.top++
	case PageUp:
		m.top -= m.h
	case PageDown:
		m.top += m.h
	case HalfPageUp:
		m.top -= m.h / 2
	case HalfPageDown:
		m.top += m.h / 2
	case Top:
		m.top = 0
	case Bottom:
		m.top = len(m.page.Lines)
	}
	m.clamp()
}

// follow scrolls the text so that the annotation under the cursor shows.
func (m *Model) follow() {
	line := m.page.Rows[m.cursor]
	if line < m.top {
		m.top = line
	}
	if m.h > 0 && line >= m.top+m.h {
		m.top = line - m.h + 1
	}
	m.clamp()
}

func (m *Model) clamp() {
	m.top = min(max(m.top, 0), max(len(m.page.Lines)-m.h, 0))
}

// View draws the window, w by h cells: a cell of margin that the cursor marks
// its row in, and the text.
func (m *Model) View() string {
	m.build()
	out := make([]string, 0, m.h)
	for i := m.top; i < min(m.top+m.h, len(m.page.Lines)); i++ {
		gutter := " "
		if len(m.page.Rows) > 0 && m.page.Rows[m.cursor] == i {
			gutter = m.opt.Styles.cursor
		}
		line := gutter + m.page.Lines[i]
		out = append(out, line+strings.Repeat(" ", max(m.w-ansi.StringWidth(line), 0)))
	}
	for len(out) < m.h {
		out = append(out, strings.Repeat(" ", max(m.w, 0)))
	}
	return strings.Join(out, "\n")
}

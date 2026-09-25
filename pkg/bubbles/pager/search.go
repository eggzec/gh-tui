package pager

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// match is where a search matched: bytes start to end of a line.
type match struct {
	line, start, end int
}

// search is the last search that was run and what it found. cur is the
// match the last jump went to.
type search struct {
	query   string
	matches []match
	cur     int
}

// Query returns the text of the search shown, or "" if there is none.
func (m Model) Query() string { return m.search.query }

// Matches returns the number of matches of the search shown.
func (m Model) Matches() int { return len(m.search.matches) }

// SetSearch searches the content for query as if the user had typed it,
// and jumps to the first match, so that a parent can open the pager on
// what it is looking for. The search is of the content shown, so set it
// after SetContent; new content clears it. An empty query clears it too.
func (m *Model) SetSearch(query string) {
	m.closeSearch()
	m.top, m.row = 0, 0
	m.runSearch(query)
}

func (m *Model) openSearch() tea.Cmd {
	m.searching = true
	m.input.Reset()
	return m.input.Focus()
}

func (m *Model) closeSearch() {
	m.searching = false
	m.input.Blur()
}

func (m *Model) clearSearch() {
	m.search = search{}
	m.enableSearchKeys()
}

func (m *Model) enableSearchKeys() {
	found := len(m.search.matches) > 0
	m.keys.Next.SetEnabled(found)
	m.keys.Prev.SetEnabled(found)
}

// runSearch finds every match of query and jumps to the first one at or
// after the top of the window. A query without capitals ignores case.
func (m *Model) runSearch(query string) {
	m.clearSearch()
	if query == "" {
		return
	}
	m.search.query = query
	pattern := regexp.QuoteMeta(query)
	if !strings.ContainsFunc(query, unicode.IsUpper) {
		pattern = "(?i)" + pattern
	}
	re := regexp.MustCompile(pattern)
	for i, l := range m.lines {
		for _, loc := range re.FindAllStringIndex(l, -1) {
			m.search.matches = append(m.search.matches, match{line: i, start: loc[0], end: loc[1]})
		}
	}
	m.enableSearchKeys()
	if len(m.search.matches) == 0 {
		return
	}
	first, _ := slices.BinarySearchFunc(m.search.matches, m.top, func(x match, line int) int {
		return x.line - line
	})
	m.jump(first % len(m.search.matches))
}

// step jumps to the next match, or the previous one for a negative d,
// wrapping around at the ends.
func (m *Model) step(d int) {
	n := len(m.search.matches)
	if n == 0 {
		return
	}
	m.jump(((m.search.cur+d)%n + n) % n)
}

// jump makes match i current and scrolls it into view: its line to the top
// unless it is shown already, and its start into the columns shown.
func (m *Model) jump(i int) {
	m.search.cur = i
	x := m.search.matches[i]
	line := m.lines[x.line]
	if x.line < m.top || x.line > m.bottom() {
		m.top, m.row = x.line, 0
	}
	tw := m.textWidth()
	if m.wrap {
		if x.line == m.top {
			m.row = m.rowOf(x.line, x.start)
		}
	} else {
		_, start := advance(line[:x.start], 0, math.MaxInt)
		_, end := advance(line[:x.end], 0, math.MaxInt)
		if start < m.left || end > m.left+tw {
			m.left = max(start-tw/4, 0)
		}
	}
	m.clamp()
}

// lineMatches returns the matches in line i, and the index of the first.
func (m Model) lineMatches(i int) (ms []match, first int) {
	all := m.search.matches
	lo, _ := slices.BinarySearchFunc(all, i, func(x match, line int) int { return x.line - line })
	hi := lo
	for hi < len(all) && all[hi].line == i {
		hi++
	}
	return all[lo:hi], lo
}

package logview

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// match is where a search matched: bytes start to end of the text of a
// row.
type match struct {
	row, start, end int
}

// search is the last search that was run and what it found. cur is the
// match the last jump went to.
type search struct {
	query   string
	re      *regexp.Regexp
	matches []match
	cur     int
}

// Query returns the text of the search shown, or "" if there is none.
func (m Model) Query() string { return m.search.query }

// Matches returns the number of matches of the search shown.
func (m Model) Matches() int { return len(m.search.matches) }

func (m *Model) openSearch() tea.Cmd {
	m.searching = true
	m.enableKeys()
	m.input.Reset()
	return m.input.Focus()
}

func (m *Model) closeSearch() {
	m.searching = false
	m.enableKeys()
	m.input.Blur()
}

func (m *Model) clearSearch() {
	m.search = search{}
	m.enableKeys()
}

// runSearch finds every match of query, in collapsed folds too, and jumps
// to the first one at or after the cursor. A query without capitals
// ignores case.
func (m *Model) runSearch(query string) {
	m.clearSearch()
	if query == "" {
		return
	}
	pattern := regexp.QuoteMeta(query)
	if !strings.ContainsFunc(query, unicode.IsUpper) {
		pattern = "(?i)" + pattern
	}
	m.search.query = query
	m.search.re = regexp.MustCompile(pattern)
	m.searchFrom(0)
	if len(m.search.matches) == 0 {
		return
	}
	first, _ := slices.BinarySearchFunc(m.search.matches, m.cursorRow(), func(x match, r int) int {
		return x.row - r
	})
	m.jump(first % len(m.search.matches))
}

// searchFrom adds the matches in the rows from row on.
func (m *Model) searchFrom(from int) {
	if m.search.re == nil {
		return
	}
	for r := from; r < len(m.rows); r++ {
		for _, loc := range m.search.re.FindAllStringIndex(m.rows[r].text, -1) {
			m.search.matches = append(m.search.matches, match{row: r, start: loc[0], end: loc[1]})
		}
	}
	m.enableKeys()
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

// jump makes match i current, expands the folds it is in, and scrolls it
// into view: its row into the window, and its start into the columns
// shown.
func (m *Model) jump(i int) {
	m.search.cur = i
	x := m.search.matches[i]
	m.reveal(x.row)
	r := &m.rows[x.row]
	if r.fold >= 0 {
		return
	}
	if m.wrap {
		if m.cur == m.top {
			m.row = m.rowOf(m.cur, x.start)
			m.clamp()
		}
		return
	}
	tw := m.textWidth(m.layout(), r)
	_, start := advance(r.text[:x.start], 0, math.MaxInt)
	_, end := advance(r.text[:x.end], 0, math.MaxInt)
	if start < m.left || end > m.left+tw {
		m.left = max(start-tw/4, 0)
	}
}

// rowMatches returns the matches in row r, and the index of the first.
func (m *Model) rowMatches(r int) (ms []match, first int) {
	all := m.search.matches
	lo, _ := slices.BinarySearchFunc(all, r, func(x match, r int) int { return x.row - r })
	hi := lo
	for hi < len(all) && all[hi].row == r {
		hi++
	}
	return all[lo:hi], lo
}

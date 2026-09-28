package pager

import (
	"context"
	"math"
	"regexp"
	"slices"

	tea "charm.land/bubbletea/v2"
)

// syncLimit is the size in bytes of content below which a search runs
// inside Update, so short files and SetSearch show their matches at once.
// Larger content is searched in a command.
const syncLimit = 256 << 10

// checkEvery is how many lines a search goes between checks that it was
// cancelled.
const checkEvery = 4096

// maxLineMatches is the most matches a search finds in one line, so a
// pattern that matches every character of a minified file costs no more
// than one that matches a word.
const maxLineMatches = 1000

// search is the last search that was run and what it found. It keeps the
// lines with a match and how many matches they hold, not the matches, so
// its memory grows with the lines of the content; where the matches are in
// a line is found only for the lines in the window ([hits]).
type search struct {
	query string
	re    *regexp.Regexp
	// invert makes the lines re doesn't match the matches, one per line,
	// marked in the gutter.
	invert bool
	// running reports whether the command that finds the matches is still
	// going.
	running bool
	// from is the line the search started at: the first match at or after
	// it is the first it jumps to, unless the window moved from top and
	// row while the search ran.
	from     int
	top, row int
	// lines are the lines with a match, in order, and ends[k] is the
	// number of matches in lines[:k+1].
	lines []int32
	ends  []int32
	// cur is the match the last jump went to, or -1 before the first:
	// match curNth of line curLine.
	cur, curLine, curNth int
}

// total returns the number of matches found.
func (s search) total() int {
	if len(s.ends) == 0 {
		return 0
	}
	return int(s.ends[len(s.ends)-1])
}

// hits are the matches of the search in the window: the lines from top to
// bottom, from row of the top one, scrolled left columns sideways, for
// search qgen.
type hits struct {
	valid                  bool
	top, bottom, row, left int
	qgen                   int
	lines                  []lineHits
}

// lineHits are the byte ranges of the matches in the part of a line the
// window shows, the first of which is match first of the line, or none
// for a line an inverted search matches.
type lineHits struct {
	line, first int
	ranges      [][]int
}

// searchMsg carries what search qgen of the pager with ID id found.
type searchMsg struct {
	id    int64
	qgen  int
	lines []int32
	ends  []int32
}

// Query returns the text of the search shown, or "" if there is none.
func (m Model) Query() string { return m.search.query }

// Matches returns the number of matches of the search shown, once it has
// found them all.
func (m Model) Matches() int { return m.search.total() }

// SetSearch searches the content for query, as it is and not as a
// pattern, and jumps to the first match, so that a parent can open the
// pager on what it is looking for: the first from the line GoToLine
// marked, if it did, or else from the start. The search is of the content
// shown, so set it after SetContent and GoToLine; new content clears it.
// An empty query clears it too. The returned command searches large
// content in the background.
func (m *Model) SetSearch(query string) tea.Cmd {
	m.closePrompt()
	from := m.mark
	if from < 0 {
		from = 0
		m.top, m.row = 0, 0
		m.clamp()
	}
	if query == "" {
		m.clearSearch()
		return nil
	}
	re, err := compile(regexp.QuoteMeta(query))
	if err != nil {
		return nil
	}
	return m.runSearch(query, re, false, from)
}

// clearSearch forgets the search, and stops it if it is still running.
func (m *Model) clearSearch() {
	if m.stopSearch != nil {
		m.stopSearch()
		m.stopSearch = nil
	}
	m.qgen++
	m.search = search{cur: -1}
	m.hits = hits{}
	m.enableSearchKeys()
}

func (m *Model) enableSearchKeys() {
	found := m.search.total() > 0
	m.keys.Next.SetEnabled(found)
	m.keys.Prev.SetEnabled(found)
	m.keys.Confirm.SetEnabled(m.prompt.Focused())
	m.keys.Cancel.SetEnabled(m.prompt.Focused() || m.search.query != "")
}

// runSearch starts a search of the content for the lines re matches, or
// doesn't match if invert is set, named query, from line from. Content
// smaller than syncLimit is searched at once; for larger content, the
// returned command searches it and the pager says it is searching until
// the matches arrive. The window shows its matches at once either way.
func (m *Model) runSearch(query string, re *regexp.Regexp, invert bool, from int) tea.Cmd {
	m.clearSearch()
	m.search = search{query: query, re: re, invert: invert, from: from, cur: -1}
	// Marks in the gutter may widen it.
	m.clamp()
	m.search.top, m.search.row = m.top, m.row
	if m.size < syncLimit {
		lines, ends, _ := find(context.Background(), re, invert, m.lines)
		m.found(lines, ends)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.stopSearch = cancel
	m.search.running = true
	m.enableSearchKeys()
	id, qgen, all := m.id, m.qgen, m.lines
	return func() tea.Msg {
		defer cancel()
		lines, ends, err := find(ctx, re, invert, all)
		if err != nil {
			return nil
		}
		return searchMsg{id: id, qgen: qgen, lines: lines, ends: ends}
	}
}

// find returns the indices of the lines that re matches, and the number of
// matches in them and every line before, until ctx is done. With invert,
// it returns the lines that re doesn't match, each a match of its own.
func find(ctx context.Context, re *regexp.Regexp, invert bool, all []string) (lines, ends []int32, err error) {
	var n int32
	for i, l := range all {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
		}
		var k int
		switch {
		case !invert:
			k = len(lineMatches(re, l))
		case !re.MatchString(l):
			k = 1
		}
		if k == 0 {
			continue
		}
		n += int32(k)
		lines = append(lines, int32(i))
		ends = append(ends, n)
	}
	return lines, ends, nil
}

// lineMatches returns the byte ranges of the matches of re in s, up to
// maxLineMatches of them, without the empty ones, which can't be shown.
func lineMatches(re *regexp.Regexp, s string) [][]int {
	all := re.FindAllStringIndex(s, maxLineMatches)
	return slices.DeleteFunc(all, func(r []int) bool { return r[0] == r[1] })
}

// found takes the lines and counts of the running search and jumps to the
// first match at or after the line it started at, or the first of all.
// If the user scrolled while it ran, it leaves the window where they took
// it, and the next step goes to the first match from there. A search that
// found nothing is cleared, with a note that says so.
func (m *Model) found(lines, ends []int32) {
	s := &m.search
	s.running, s.lines, s.ends = false, lines, ends
	m.stopSearch = nil
	if len(lines) == 0 {
		m.clearSearch()
		m.clamp()
		m.flash = noteNotFound
		return
	}
	m.enableSearchKeys()
	if m.top != s.top || m.row != s.row {
		return
	}
	m.jump(m.firstFrom(s.from))
}

// firstFrom returns the first match at or after line i, or the first of
// all past the last.
func (m Model) firstFrom(i int) int {
	s := m.search
	k, _ := slices.BinarySearch(s.lines, int32(i))
	if k == 0 || k == len(s.lines) {
		return 0
	}
	return int(s.ends[k-1])
}

// step jumps to the next match, or the previous one for a negative d,
// wrapping around at the ends. Before the first jump, it goes to the
// first match from the top of the window, or the last before it.
func (m *Model) step(d int) {
	n := m.search.total()
	switch {
	case n == 0:
	case m.search.cur >= 0:
		m.jump(((m.search.cur+d)%n + n) % n)
	case d > 0:
		m.jump(m.firstFrom(m.top))
	default:
		m.jump((m.firstFrom(m.top) - 1 + n) % n)
	}
}

// jump makes match i current and scrolls it into view: its line to the top
// unless it is shown already, and its start into the columns shown.
func (m *Model) jump(i int) {
	s := &m.search
	k, _ := slices.BinarySearch(s.ends, int32(i+1))
	nth := i
	if k > 0 {
		nth -= int(s.ends[k-1])
	}
	s.cur, s.curLine, s.curNth = i, int(s.lines[k]), nth
	if s.curLine < m.top || s.curLine > m.bottom() {
		m.top, m.row = s.curLine, 0
	}
	if s.invert {
		m.clamp()
		return
	}
	line := m.lines[s.curLine]
	ranges := lineMatches(s.re, line)
	if len(ranges) <= nth {
		// The count and the ranges come from the same regexp, so only
		// content changed behind the search's back gets here.
		m.clamp()
		return
	}
	start, end := ranges[nth][0], ranges[nth][1]
	tw := m.textWidth()
	if m.wrap {
		if s.curLine == m.top {
			m.row = m.rowOf(s.curLine, start)
		}
	} else {
		_, from := advance(line[:start], 0, math.MaxInt)
		_, to := advance(line[:end], 0, math.MaxInt)
		if from < m.left || to > m.left+tw {
			m.left = max(from-tw/4, 0)
		}
	}
	m.clamp()
}

// findHits finds the matches in the part of each line the window shows,
// unless it has them already.
func (m *Model) findHits() {
	re := m.search.re
	if re == nil || len(m.lines) == 0 {
		m.hits = hits{}
		return
	}
	top, bottom := m.top, m.bottom()
	if h := m.hits; h.valid && h.qgen == m.qgen && h.top == top && h.bottom == bottom &&
		h.row == m.row && h.left == m.left {
		return
	}
	var lines []lineHits
	for i := top; i <= bottom; i++ {
		if m.search.invert {
			if !re.MatchString(m.lines[i]) {
				lines = append(lines, lineHits{line: i})
			}
			continue
		}
		all := lineMatches(re, m.lines[i])
		a, e := m.shown(i)
		first, _ := slices.BinarySearchFunc(all, a+1, func(r []int, pos int) int { return r[1] - pos })
		end := first
		for end < len(all) && all[end][0] < e {
			end++
		}
		if end > first {
			// A copy, so the matches out of view go.
			lines = append(lines, lineHits{line: i, first: first, ranges: slices.Clone(all[first:end])})
		}
	}
	m.hits = hits{valid: true, top: top, bottom: bottom, row: m.row, left: m.left, qgen: m.qgen, lines: lines}
}

// shown returns the bytes a to e of line i that the window shows.
func (m Model) shown(i int) (a, e int) {
	s, tw := m.lines[i], m.textWidth()
	if !m.wrap {
		a, _ = m.leftEdge(s)
		e, _ = advance(s, a, tw)
		return a, e
	}
	if i == m.top {
		for range m.row {
			a, _ = nextRow(s, a, tw)
		}
	}
	e = a
	for r := 0; r < m.bodyHeight() && e < len(s); r++ {
		e, _ = nextRow(s, e, tw)
	}
	return a, e
}

// lineHits returns the byte ranges of the matches in the part of line i
// that the window shows, the index in the line of the first, and whether
// the line is a match at all, which a line of an inverted search is as a
// whole.
func (m Model) lineHits(i int) (ranges [][]int, first int, ok bool) {
	ls := m.hits.lines
	k, ok := slices.BinarySearchFunc(ls, i, func(h lineHits, line int) int { return h.line - line })
	if !ok {
		return nil, 0, false
	}
	return ls[k].ranges, ls[k].first, true
}

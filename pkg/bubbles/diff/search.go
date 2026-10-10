package diff

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// match is where a search matched: bytes start to end of the cleaned text
// (the text as drawn, with tabs expanded) of body row row of file file.
type match struct {
	file, row  int
	start, end int
}

// search is the last search that was run and what it found, in the order
// of the rows: across the files fetched so far, which a search that is
// shown keeps up with as pages arrive.
type search struct {
	query   string
	re      *regexp.Regexp
	matches []match
	// cur is the match the last jump went to, or -1 while none has been
	// jumped to: a search that found nothing in the files fetched so far
	// has no current match when the first ones arrive, if a key press
	// ended the wait.
	cur int
	// files is how many files the matches were looked for in.
	files int
	// waiting says that nothing matched in the files fetched so far, so
	// the first match to arrive is jumped to, until a key press moves the
	// cursor.
	waiting bool
}

// hit is a match in the line a row shows, in the bytes of its cleaned
// text.
type hit struct {
	start, end int
	current    bool
}

// Query returns the text of the search shown, or "" if there is none.
func (m Model) Query() string { return m.search.query }

// Matches returns the number of matches of the search shown, in the files
// fetched so far.
func (m Model) Matches() int { return len(m.search.matches) }

// Capturing reports whether the search input is open. It then takes every
// key, so the parent should not act on keys of its own.
func (m Model) Capturing() bool { return m.searching }

// ClearSearch forgets the search shown, and reports whether there was one.
// The parent calls it for the key that dismisses, which clears the search
// before it closes anything.
func (m *Model) ClearSearch() bool {
	if m.search.query == "" {
		return false
	}
	m.clearSearch()
	return true
}

func (m *Model) openSearch() tea.Cmd {
	m.searching = true
	m.syncKeys()
	m.input.Reset()
	return m.input.Focus()
}

func (m *Model) closeSearch() {
	m.searching = false
	m.syncKeys()
	m.input.Blur()
}

func (m *Model) clearSearch() {
	m.search = search{}
	m.syncKeys()
}

// pressSearch takes a key while the search input is open.
func (m *Model) pressSearch(k tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(k, m.keyMap.Confirm):
		query := m.input.Value()
		m.closeSearch()
		m.runSearch(query)
		return m.sync()
	case key.Matches(k, m.keyMap.Cancel),
		m.input.Value() == "" && key.Matches(k, m.keyMap.CancelEmpty):
		m.closeSearch()
		return nil
	}
	// The input edits its text in place, which copies of the model share,
	// so it gets a copy of its own first.
	m.input.SetValue(m.input.Value())
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return cmd
}

// runSearch finds every match of query in the lines of the files fetched,
// and jumps to the first at or after the cursor. A query without capitals
// ignores case. The pages still to come are fetched, and searched as they
// arrive.
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
	m.search.cur = -1
	m.searchFrom(0)
	if len(m.search.matches) == 0 {
		m.search.waiting = !m.pg.done
		return
	}
	m.jump(m.matchFrom(false) % len(m.search.matches))
}

// searchFrom adds the matches in the files from file from on.
func (m *Model) searchFrom(from int) {
	if m.search.re == nil {
		return
	}
	for f := from; f < m.layout.Files(); f++ {
		m.layout.ensure(f)
		for i, r := range m.layout.files[f].body {
			if !searchable(r.Kind) {
				continue
			}
			s := termtext.Clean(r.Text, m.tabs)
			for _, loc := range m.search.re.FindAllStringIndex(s, -1) {
				m.search.matches = append(m.search.matches, match{file: f, row: i, start: loc[0], end: loc[1]})
			}
		}
	}
	m.search.files = m.layout.Files()
	m.syncKeys()
}

// searchMore searches the files that arrived since the last search, and
// jumps to the first match if the search waited for one.
func (m *Model) searchMore() {
	if m.search.re == nil || m.search.files >= m.layout.Files() {
		return
	}
	m.searchFrom(m.search.files)
	if m.search.waiting && len(m.search.matches) > 0 {
		m.search.waiting = false
		m.jump(m.matchFrom(false) % len(m.search.matches))
	}
}

// rebuildSearch finds the matches again, for lines that read differently.
func (m *Model) rebuildSearch() {
	if m.search.re == nil {
		return
	}
	m.search.matches, m.search.files = nil, 0
	m.searchFrom(0)
	m.search.cur = min(m.search.cur, len(m.search.matches)-1)
}

// searchable reports whether the rows of kind k are lines of the diff,
// which a search reads: not the headers, the notes, nor the marker of a
// missing newline.
func searchable(k Kind) bool {
	switch k {
	case KindContext, KindAdded, KindDeleted, KindRaw:
		return true
	case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote:
	}
	return false
}

// matchFrom returns the index of the first match at or after the cursor's
// row, or after it if after is set. It is the length of the matches when
// there is none, which wraps to the first.
func (m *Model) matchFrom(after bool) int {
	f, ok := m.layout.FileAt(m.cursor)
	if !ok {
		return 0
	}
	start, _ := m.layout.FileRow(f)
	// The header of a file stands before its first row.
	at := [2]int{f, m.cursor - start - 1}
	i, _ := slices.BinarySearchFunc(m.search.matches, at, func(x match, at [2]int) int {
		if x.file != at[0] {
			return x.file - at[0]
		}
		return x.row - at[1]
	})
	for after && i < len(m.search.matches) && m.search.matches[i].file == at[0] && m.search.matches[i].row == at[1] {
		i++
	}
	return i
}

// onCurrent reports whether the cursor is on the row of the current match.
func (m Model) onCurrent() bool {
	if m.search.cur < 0 || m.search.cur >= len(m.search.matches) {
		return false
	}
	x := m.search.matches[m.search.cur]
	start, _ := m.layout.FileRow(x.file)
	return !m.layout.Collapsed(x.file) && m.cursor == start+1+x.row
}

// step jumps to the next match, or the previous one for a negative d,
// wrapping round at the ends. From the current match it goes on in the
// order of the matches; when the cursor has left it, or no match was jumped
// to yet, it goes on from the cursor, to the first match after it, or the
// last one before it, as a search does in less.
func (m *Model) step(d int) {
	n := len(m.search.matches)
	if n == 0 {
		return
	}
	if m.onCurrent() {
		m.jump(((m.search.cur+d)%n + n) % n)
		return
	}
	if d > 0 {
		// With no current match, the cursor's own row counts.
		m.jump(m.matchFrom(m.search.cur >= 0) % n)
		return
	}
	m.jump((m.matchFrom(false) - 1 + n) % n)
}

// jump makes match i current, unfolds its file if it is folded, and
// scrolls it into view: its row into the window, and its start into the
// columns shown.
func (m *Model) jump(i int) {
	m.search.cur = i
	x := m.search.matches[i]
	m.layout.SetCollapsed(x.file, false)
	start, _ := m.layout.FileRow(x.file)
	row := start + 1 + x.row
	m.cursor = row
	if h := m.bodyHeight(); row < m.top || row >= m.top+h {
		m.top = row - h/2
	}
	m.dirty = true
	m.scroll()
	r, ok := m.layout.RowAt(row)
	if !ok {
		return
	}
	s := termtext.Clean(r.Text, m.tabs)
	from, to := ansi.StringWidth(s[:x.start]), ansi.StringWidth(s[:x.end])
	room := max(m.textWidth(m.numWidth())-1, 1)
	if from < m.left || to > m.left+room {
		m.left = max(from-room/4, 0)
	}
	m.left = min(m.left, m.maxLeft())
}

// rowHits returns the matches in the line that row shows, which is row
// number index of the layout, or nil if it has none.
func (m Model) rowHits(row Row, index int) []hit {
	all := m.search.matches
	if len(all) == 0 || !searchable(row.Kind) {
		return nil
	}
	start, _ := m.layout.FileRow(row.File)
	body := index - start - 1
	lo, _ := slices.BinarySearchFunc(all, [2]int{row.File, body}, func(x match, at [2]int) int {
		if x.file != at[0] {
			return x.file - at[0]
		}
		return x.row - at[1]
	})
	var hits []hit
	for i := lo; i < len(all) && all[i].file == row.File && all[i].row == body; i++ {
		hits = append(hits, hit{start: all[i].start, end: all[i].end, current: i == m.search.cur})
	}
	return hits
}

// paint writes s, a line of text of the kind t styles, in the colors of its
// spans, with the matches in hits over them: those in match, and the
// current one in current. What lies outside them looks as coloured has it.
func (t tokenStyles) paint(s string, spans []syntax.Span, hits []hit, match, current sgr) string {
	if len(hits) == 0 {
		return t.coloured(s, spans)
	}
	var b strings.Builder
	b.Grow(len(s) + 24*(len(spans)+2*len(hits)))
	pos, si, hi := 0, 0, 0
	for pos < len(s) {
		for si < len(spans) && spans[si].End <= pos {
			si++
		}
		for hi < len(hits) && hits[hi].end <= pos {
			hi++
		}
		style, end := t.text, len(s)
		if si < len(spans) {
			style, end = t.token(spans[si].Type), min(spans[si].End, len(s))
		}
		if hi < len(hits) {
			switch h := hits[hi]; {
			case h.start <= pos:
				style, end = match, min(end, h.end)
				if h.current {
					style = current
				}
			default:
				end = min(end, h.start)
			}
		}
		b.WriteString(style.on(s[pos:end]))
		pos = end
	}
	return b.String()
}

// newInput returns the input of the search.
func newInput() textinput.Model {
	in := textinput.New()
	in.Prompt = "/"
	return in
}

// searchStatus is what the status line says of the search shown: which
// match the cursor went to among those found, with a "+" while more pages
// are to come, or that there are none.
func (m Model) searchStatus() (text string, none bool) {
	switch n := len(m.search.matches); {
	case m.search.query == "":
		return "", false
	case n > 0:
		more := ""
		if !m.pg.done {
			more = "+"
		}
		switch {
		case m.search.cur < 0 && n == 1:
			return "1 match" + more, false
		case m.search.cur < 0:
			return strconv.Itoa(n) + " matches" + more, false
		}
		return "match " + strconv.Itoa(m.search.cur+1) + "/" + strconv.Itoa(n) + more, false
	case !m.pg.done && m.pg.err == nil:
		return "searching" + m.styles.Ellipsis, false
	}
	return "no matches", true
}

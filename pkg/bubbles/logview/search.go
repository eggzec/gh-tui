package logview

import (
	"math"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/internal/wrap"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// rowStore gives the pager, which does the search, the text of the rows. The
// model and its copies share it, and it follows the rows of the model.
type rowStore struct {
	rows []row
}

func (s *rowStore) Len() int { return len(s.rows) }

func (s *rowStore) Text(i int) string {
	if i < 0 || i >= len(s.rows) {
		return ""
	}
	return s.rows[i].text
}

// Marks is nil, since the pager only searches the rows: this view draws them.
func (s *rowStore) Marks(int) []termtext.Style { return nil }

// Query returns the text of the search shown, or "" if there is none.
func (m Model) Query() string { return m.pg.Query() }

// Matches returns the number of matches of the search shown.
func (m Model) Matches() int { return m.pg.Matches() }

// takeRows gives the pager the rows of the model, which it searches from its
// start.
func (m *Model) takeRows() {
	m.store.rows = m.rows
	m.pg.SetRows(m.store)
	m.enableKeys()
}

// growRows tells the pager of the rows appended from row from, which it
// searches for the matches of the search shown.
func (m *Model) growRows(from int) {
	m.store.rows = m.rows
	m.pg.Append(from)
	m.enableKeys()
}

func (m *Model) openSearch() tea.Cmd {
	cmd := m.pg.OpenSearch()
	m.enableKeys()
	return cmd
}

func (m *Model) closeSearch() {
	m.pg.CancelPrompt()
	m.enableKeys()
}

// syncPosition tells the pager where the cursor is, since a search starts
// from there.
func (m *Model) syncPosition() {
	m.pg.SetPosition(pager.Position{Top: m.cursorRow()})
}

// seekCursor moves a search just run on to the first match at or after the
// cursor. The pager starts from the top of its window, which a short log
// leaves above the cursor.
func (m *Model) seekCursor() {
	m.pg.JumpFrom(m.cursorRow())
}

// step jumps to the next match, or the previous one for a negative d,
// wrapping around at the ends. Before a search has jumped to a match, it
// goes from the cursor.
func (m *Model) step(d int) {
	m.syncPosition()
	m.pg.StepMatchFrom(d, m.cursorRow())
	m.syncMatch()
}

// syncMatch follows the pager to its current match: it expands the folds the
// match is in, and scrolls it into view: its row into the window, and its
// start into the columns shown.
func (m *Model) syncMatch() {
	m.enableKeys()
	line, ok := m.pg.Current()
	if !ok || line >= len(m.rows) {
		return
	}
	m.reveal(line)
	r := &m.rows[line]
	if r.fold >= 0 {
		return
	}
	ranges, cur := m.pg.Ranges(line)
	if cur < 0 || cur >= len(ranges) {
		return
	}
	start, end := ranges[cur][0], ranges[cur][1]
	if m.wrap {
		if m.cur == m.top {
			m.row = m.rowOf(m.cur, start)
			m.clamp()
		}
		return
	}
	tw := m.textWidth(m.layout(), r)
	_, from := wrap.Advance(r.text[:start], 0, math.MaxInt)
	_, to := wrap.Advance(r.text[:end], 0, math.MaxInt)
	if from < m.left || to > m.left+tw {
		m.left = max(from-tw/4, 0)
	}
}

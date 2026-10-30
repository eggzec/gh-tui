package logview

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

// rowStore gives the pager, which does the search, the text of the rows. The
// model and its copies share it, and it follows the rows of the model.
type rowStore struct {
	rows []row
	look look
	// vis is the rows shown, as the model last told the pager.
	vis []int
	// revealed is set when a search opened folds through reveal: look.folds
	// and vis then hold the folds and rows the model takes back.
	revealed bool
}

// anchor returns the row shown that stands in for row r: the header of the
// collapsed fold that hides it.
func (s *rowStore) anchor(r int) int {
	return s.vis[locate(s.vis, s.rows, s.look.folds, r)]
}

// reveal opens the folds that hide row r, which a search lands on, and
// returns the rows shown then.
func (s *rowStore) reveal(r int) ([]int32, bool) {
	if r < 0 || r >= len(s.rows) {
		return nil, false
	}
	folds := slices.Clone(s.look.folds)
	for f := s.rows[r].parent; f >= 0; f = folds[f].parent {
		folds[f].open = true
	}
	s.look.folds = folds
	s.vis = visible(s.rows, folds, len(s.vis))
	s.revealed = true
	return mask(s.vis, len(s.rows)), true
}

// mask returns vis, the rows shown of n, as the pager takes it: nil when
// every row is shown.
func mask(vis []int, n int) []int32 {
	if len(vis) == n {
		return nil
	}
	out := make([]int32, len(vis))
	for i, r := range vis {
		out[i] = int32(r)
	}
	return out
}

// pushVisible tells the pager which rows the folds show.
func (m *Model) pushVisible() {
	m.syncLook()
	m.store.vis = m.vis
	m.pg.SetVisible(mask(m.vis, len(m.rows)), m.store.anchor)
}

// adoptReveal takes back the folds a search opened through the store.
func (m *Model) adoptReveal() {
	if s := m.store; s.revealed {
		m.folds, m.vis = s.look.folds, s.vis
		s.revealed = false
	}
}

func (s *rowStore) Len() int { return len(s.rows) }

func (s *rowStore) Text(i int) string {
	if i < 0 || i >= len(s.rows) {
		return ""
	}
	return s.rows[i].text
}

// Query returns the text of the search shown, or "" if there is none.
func (m Model) Query() string { return m.pg.Query() }

// Matches returns the number of matches of the search shown.
func (m Model) Matches() int { return m.pg.Matches() }

// takeRows gives the pager the rows of the model, which it searches from its
// start.
func (m *Model) takeRows() {
	m.syncLook()
	m.pg.SetRows(m.store)
	m.pushVisible()
	m.enableKeys()
}

// growRows tells the pager of the rows appended from row from, which it
// searches for the matches of the search shown.
func (m *Model) growRows(from int) {
	m.syncLook()
	m.pg.Append(from)
	m.pushVisible()
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

// seekCursor moves a search just run on to the first match at or after row
// from, where the cursor was. The pager starts from the top of its window,
// which may be above the cursor.
func (m *Model) seekCursor(from int) {
	m.syncLook()
	m.pg.JumpFrom(from)
}

// step jumps to the next match, or the previous one for a negative d,
// wrapping around at the ends. Before a search has jumped to a match, it
// goes from the cursor.
func (m *Model) step(d int) {
	m.syncLook()
	m.pg.StepMatchFrom(d, m.cursorRow())
	m.syncMatch()
}

// syncMatch takes back the folds the pager opened to land on a match; the
// pager moved the cursor and the window to it.
func (m *Model) syncMatch() {
	m.adoptReveal()
	m.enableKeys()
}

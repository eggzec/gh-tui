package pager

import "github.com/eggzec/gh-tui/pkg/termtext"

// Rows is what the pager shows: one entry per row before folding and
// wrapping. The pager reads it while it draws, searches and measures, and
// never changes it. A host that keeps its rows in a store of its own gives
// the pager this view of them, and must not change them between a call to
// [Model.SetRows] and the next, or the pager shows what it held when they
// changed.
type Rows interface {
	// Len returns the number of rows.
	Len() int
	// Text returns the text of row i, cleaned: tabs expanded, no escape
	// sequences and no newline. A host whose Len shrinks returns "" for an
	// index that is out of range. The pager scans Text once, at SetRows,
	// to learn the size of the rows, so it should be cheap.
	Text(i int) string
	// Marks returns the colors of row i, as [termtext.CleanStyled] splits
	// them from the text of the row, or nil when the row is plain. A host
	// whose rows are colored returns a non-nil empty slice for a row with
	// no color, such as an empty one, so that it is drawn as the others
	// are. Marks is asked more than once for each row drawn and must not
	// allocate.
	Marks(i int) []termtext.Style
}

// WithRows shows the rows of r from the start, in place of content. The
// pager does not highlight them.
func WithRows(r Rows) Option {
	return func(s *settings) { s.rows = r }
}

// SetRows shows the rows of r in place of the content, from the first row,
// and forgets the search, the filter and the place in the old content, as
// [Model.SetContent] does.
func (m *Model) SetRows(r Rows) {
	m.dropDocs()
	m.reset(m.name, stateReady, nil)
	m.takeRows(r)
}

// takeRows shows the rows of r, which the pager has just reset to take.
func (m *Model) takeRows(r Rows) {
	if r == nil {
		r = noRows
	}
	m.rows, m.state = r, stateReady
	m.size = 0
	for i := range r.Len() {
		m.size += len(r.Text(i)) + 1
	}
	if m.proj.squeeze {
		_ = m.project(m.proj)
	}
	m.clamp()
}

// sliceRows is the rows of the content given to SetContent: its lines, and
// the colors of its own that they have, one entry for each line, or none.
// It reads without allocating, so the pager draws as fast as it did from
// the slices.
type sliceRows struct {
	lines []string
	sgr   [][]termtext.Style
}

// noRows has no row. It is never changed, so the pagers share it.
var noRows = &sliceRows{}

func (r *sliceRows) Len() int          { return len(r.lines) }
func (r *sliceRows) Text(i int) string { return r.lines[i] }

func (r *sliceRows) Marks(i int) []termtext.Style {
	if r.sgr == nil {
		return nil
	}
	if m := r.sgr[i]; m != nil {
		return m
	}
	// Colored content draws every row through its colors, an empty row
	// too, so it says "colored, nothing to color" and not "plain".
	return noMarks
}

// noMarks is the marks of a row of colored content that has none.
var noMarks = []termtext.Style{}

// plainLines returns the lines of the content that has no colors of its
// own, which the highlighter reads, or false for rows of a host and for
// colored content.
func (m Model) plainLines() ([]string, bool) {
	r, ok := m.rows.(*sliceRows)
	if !ok || r.sgr != nil {
		return nil, false
	}
	return r.lines, true
}

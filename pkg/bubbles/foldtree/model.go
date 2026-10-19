// Package foldtree is a tree of foldable, multi-line rows with guides, a
// cursor that follows a row through data refreshes, and the same keys in
// every pane that shows one.
//
// The caller owns what a row says: it hands over rows in display order, each
// with its depth and its text already rendered, and the tree owns what the
// rows have in common. That is the cursor, the folds, the scroll window, the
// guides that hang children off their parent, the ancestors pinned above a
// scrolled window, and the fold markers at the end of a closed row.
package foldtree

import (
	"maps"
	"slices"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/x/ansi"
)

// tabWidth is the stop that tabs in the text of a row expand to, so a tab
// never shifts a guide or the right-aligned part of a row.
const tabWidth = 4

// guideWidth is the cells of one level of guides.
const guideWidth = 3

// Row is one row of a tree. Rows are given in display order; a row's
// children are the rows after it that are deeper than it.
type Row struct {
	// ID names the row. It must be unique and keep naming the same row
	// when the rows are set again, since the cursor and the folds follow
	// it.
	ID string
	// Depth is the level of the row, 0 at the top. A negative depth is 0.
	Depth int
	// Lines is the text of the row, already rendered and styled: the first
	// line is the row itself and the others are its body, which a closed
	// fold hides. Text from outside must be cleaned first (see
	// termtext.Clean). Tabs are expanded to stops of 4, and a line that
	// holds a newline is split in two.
	Lines []string
	// Marker is what a closed fold hides, such as "⋯ 3 replies", drawn dim
	// at the end of the first line after two spaces and before Right. It
	// is dropped when the text leaves it fewer than 8 columns.
	Marker string
	// Right is the part drawn at the right edge of the first line, such as
	// an age. A row that is too long cuts its text first and drops Right
	// only when it alone does not fit.
	Right string
	// Fold marks a row that folds: its children, and its body lines after
	// the first, show only while it is open.
	Fold bool
	// Closed is the state of a fold the first time the tree sees its ID.
	// After that the tree keeps the state the user left it in.
	Closed bool
	// Arrow draws a dim ▸ or ▾ before the text of a fold.
	Arrow bool
	// Action marks a row whose enter does something of its own, which the
	// tree reports with an [EnterMsg]. Enter on a fold row without an
	// action toggles the fold.
	Action bool
	// Skip marks a row the cursor never rests on, such as a rule between
	// sections.
	Skip bool
	// Kind groups the rows that space marks one after another, such as
	// "file" or "commit". A row without a kind has nothing to mark.
	Kind string
	// Inline marks a row that hangs off its parent as part of the parent's
	// own text instead of as one of its children: the parent's closed fold
	// hides it as it hides a child, and a deeper row is its child in turn, but
	// it is drawn under the guides of the parent's body, with no branch of its
	// own and no guide width, so its text starts where the body of the parent
	// starts. The nearest row above that is not inline is the parent.
	Inline bool
}

// nextID gives each tree an identity that its messages carry.
var nextID atomic.Int64

// Model is a tree. Make one with [New].
type Model struct {
	id      int
	keyMap  KeyMap
	styles  Styles
	look    look
	width   int
	height  int
	focused bool

	rows   []Row
	closed map[string]bool

	// vis lists the indexes in rows of the rows that show, and starts the
	// first line of each of them in the whole text, with the total at the
	// end. first and body are the guides of their first line and of the
	// rest, in the same order, and bodyw the width in cells of body.
	vis    []int
	starts []int
	first  []string
	body   []string
	bodyw  []int

	// pins lists the places in vis of the ancestors of the top row of the
	// window that stay pinned above it, the nearest last.
	pins []int

	// sel is the ID of the row under the cursor and cur its place in vis;
	// off is the line of the text at the top of the window.
	sel string
	cur int
	off int

	// bottom asks the next [Model.scroll] to show the end of the cursor row
	// when it is taller than the window, as moving up into it does.
	bottom bool
}

// New returns a tree with no rows. It starts blurred and takes no key until
// [WithKeyMap] gives it keys.
func New(opts ...Option) Model {
	m := Model{
		id:     int(nextID.Add(1)),
		keyMap: NewKeyMap(unbound),
		closed: map[string]bool{},
	}
	m.setStyles(DefaultStyles(true))
	for _, o := range opts {
		o(&m)
	}
	m.reflow()
	return m
}

// ID returns the identity that the messages of the tree carry.
func (m Model) ID() int { return m.id }

// Len returns the number of rows that show, folds hiding the rest.
func (m Model) Len() int { return len(m.vis) }

// Focused reports whether the tree takes keys and draws its cursor row.
func (m Model) Focused() bool { return m.focused }

// Focus makes the tree take keys.
func (m *Model) Focus() { m.focused = true }

// Blur makes the tree ignore keys.
func (m *Model) Blur() { m.focused = false }

// Width returns the width of the tree in cells.
func (m Model) Width() int { return m.width }

// Height returns the height of the tree in cells.
func (m Model) Height() int { return m.height }

// SetSize sets the width and height of the tree in cells and keeps the
// cursor in view.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.scroll()
}

// KeyMap returns the key bindings of the tree.
func (m Model) KeyMap() KeyMap { return m.keyMap }

// SetKeyMap sets the key bindings of the tree.
func (m *Model) SetKeyMap(k KeyMap) { m.keyMap = k }

// Styles returns the styles of the tree.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles of the tree.
func (m *Model) SetStyles(s Styles) {
	m.setStyles(s)
	m.reflow()
}

// Selected returns the ID of the row under the cursor, and false when there
// is no row.
func (m Model) Selected() (id string, ok bool) {
	if len(m.vis) == 0 {
		return "", false
	}
	return m.rows[m.vis[m.cur]].ID, true
}

// Folded reports whether the row with the given ID is a fold that is
// closed.
func (m Model) Folded(id string) bool { return m.closed[id] }

// SetRows replaces the rows. The cursor stays on the row it was on, by ID,
// wherever that row went; if the row is gone it goes to the row now where it
// was, and if it is hidden by a closed fold, to that fold. The folds keep the
// state the user left them in. The tree keeps its own copy of rows.
func (m *Model) SetRows(rows []Row) {
	next := make([]Row, len(rows))
	closed := make(map[string]bool, len(m.closed))
	for i, r := range rows {
		r.Depth = max(r.Depth, 0)
		r.Lines = tidy(r.Lines)
		next[i] = r
		if !r.Fold {
			continue
		}
		c, seen := m.closed[r.ID]
		if !seen {
			c = r.Closed
		}
		closed[r.ID] = c
	}
	m.rows, m.closed = next, closed
	m.reflow()
}

// Reveal opens the folds above the row with the given ID, puts the cursor on
// it and scrolls to it. It reports whether there is such a row.
func (m *Model) Reveal(id string) bool {
	r := m.rowIndex(id)
	if r < 0 {
		return false
	}
	m.open(r)
	m.reflow()
	m.setCur(m.selectable(m.visIndex(id), 1))
	m.scroll()
	return true
}

// SetFolded opens the fold with the given ID, or closes it when closed is
// true, and keeps the cursor on its row, or on the fold that now hides it. It
// does nothing when there is no such fold.
func (m *Model) SetFolded(id string, closed bool) {
	r := m.rowIndex(id)
	if r < 0 || !m.rows[r].Fold || m.closed[id] == closed {
		return
	}
	m.closed = withKey(m.closed, id, closed)
	m.reflow()
}

// open opens the closed folds above row r.
func (m *Model) open(r int) {
	need := m.rows[r].Depth
	for k := r - 1; k >= 0 && need > 0; k-- {
		if p := m.rows[k]; p.Depth < need {
			need = p.Depth
			if p.Fold && m.closed[p.ID] {
				m.closed = withKey(m.closed, p.ID, false)
			}
		}
	}
}

// withKey returns a copy of c with k set to v, since models are values that
// share their maps.
func withKey(c map[string]bool, k string, v bool) map[string]bool {
	out := make(map[string]bool, len(c))
	maps.Copy(out, c)
	out[k] = v
	return out
}

// tidy returns lines with tabs expanded and no newline inside a line, and at
// least one line, copying only when it has to.
func tidy(lines []string) []string {
	if len(lines) == 0 {
		return []string{""}
	}
	clean := true
	for _, l := range lines {
		if strings.ContainsAny(l, "\t\n") {
			clean = false
			break
		}
	}
	if clean {
		return slices.Clone(lines)
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		for part := range strings.SplitSeq(l, "\n") {
			out = append(out, expandTabs(part))
		}
	}
	return out
}

// expandTabs expands the tabs of s to stops of [tabWidth] cells.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	for i, part := range strings.Split(s, "\t") {
		if i > 0 {
			col := ansi.StringWidth(b.String())
			b.WriteString(strings.Repeat(" ", tabWidth-col%tabWidth))
		}
		b.WriteString(part)
	}
	return b.String()
}

// rowIndex returns the place in rows of the row with the ID, or -1.
func (m Model) rowIndex(id string) int {
	for i := range m.rows {
		if m.rows[i].ID == id {
			return i
		}
	}
	return -1
}

// visIndex returns the place in vis of the row with the ID, or -1.
func (m Model) visIndex(id string) int {
	for i, r := range m.vis {
		if m.rows[r].ID == id {
			return i
		}
	}
	return -1
}

// row returns the row at place i of vis.
func (m Model) row(i int) Row { return m.rows[m.vis[i]] }

// shut reports whether row r is a fold that hides what follows it.
func (m Model) shut(r Row) bool { return r.Fold && m.closed[r.ID] }

// lines returns the number of lines the row at place i of vis takes.
func (m Model) lines(i int) int { return m.starts[i+1] - m.starts[i] }

// reflow works out what shows, where each row starts and its guides, then
// puts the cursor back on its row and in view.
func (m *Model) reflow() {
	vis := make([]int, 0, len(m.rows))
	for i := 0; i < len(m.rows); i++ {
		vis = append(vis, i)
		if r := m.rows[i]; m.shut(r) {
			for i+1 < len(m.rows) && m.rows[i+1].Depth > r.Depth {
				i++
			}
		}
	}
	m.vis = vis

	m.starts = make([]int, len(vis)+1)
	for i := range vis {
		n := len(m.rows[vis[i]].Lines)
		if m.shut(m.rows[vis[i]]) {
			n = 1
		}
		m.starts[i+1] = m.starts[i] + n
	}
	m.guides()
	m.place()
	m.scroll()
}

// guides draws the guides of every shown row. It walks from the last row up,
// remembering for each level whether a row of that level follows before the
// level ends, which is what tells a branch from the end of one.
func (m *Model) guides() {
	m.first = make([]string, len(m.vis))
	m.body = make([]string, len(m.vis))
	m.bodyw = make([]int, len(m.vis))
	depth := 0
	for _, r := range m.vis {
		depth = max(depth, m.rows[r].Depth)
	}
	more := make([]bool, depth+2)
	// next is the depth of the nearest row below that is not inline, or -1:
	// inline rows are not children as far as the guides go.
	next := -1
	for i := len(m.vis) - 1; i >= 0; i-- {
		if m.rows[m.vis[i]].Inline {
			continue
		}
		d := m.rows[m.vis[i]].Depth
		m.first[i], m.body[i], m.bodyw[i] = "", "", guideWidth*d
		if d > 0 {
			var pre strings.Builder
			for k := 1; k < d; k++ {
				pre.WriteString(m.look.pick(more[k], m.look.line, m.look.none))
			}
			m.first[i] = pre.String() + m.look.pick(more[d], m.look.branch, m.look.last)
			m.body[i] = pre.String() + m.look.pick(more[d], m.look.line, m.look.none)
		}
		// The body of a row that has children carries the guide on to them.
		if next > d {
			m.body[i] += m.look.line
			m.bodyw[i] += guideWidth
		}
		next = d
		more[d] = true
		for k := d + 1; k < len(more); k++ {
			more[k] = false
		}
	}
	// An inline row takes the body of the row it hangs off.
	owner := -1
	for i, r := range m.vis {
		switch {
		case !m.rows[r].Inline:
			owner = i
		case owner >= 0:
			m.first[i], m.body[i], m.bodyw[i] = m.body[owner], m.body[owner], m.bodyw[owner]
		}
	}
}

// indent returns the cells the guides of the row at place i of vis take before
// its text.
func (m Model) indent(i int) int {
	if r := m.row(i); r.Inline {
		return m.bodyw[i]
	}
	return guideWidth * m.row(i).Depth
}

// place puts the cursor on its row after the rows or the folds changed.
func (m *Model) place() {
	if len(m.vis) == 0 {
		m.cur, m.sel = 0, ""
		return
	}
	if i := m.visIndex(m.sel); i >= 0 {
		m.cur = i
		return
	}
	if r := m.rowIndex(m.sel); r >= 0 {
		// The row is hidden by a closed fold, which is the last row
		// shown before it.
		m.cur = max(sort.SearchInts(m.vis, r)-1, 0)
	}
	m.cur = min(max(m.cur, 0), len(m.vis)-1)
	m.setCur(m.selectable(m.cur, 1))
}

// selectable returns the place of the nearest row the cursor may rest on,
// looking from i in direction dir first and the other way after.
func (m Model) selectable(i, dir int) int {
	for _, d := range []int{dir, -dir} {
		for j := i; j >= 0 && j < len(m.vis); j += d {
			if !m.row(j).Skip {
				return j
			}
		}
	}
	return i
}

// setCur puts the cursor on the row at place i of vis.
func (m *Model) setCur(i int) {
	if i < 0 || i >= len(m.vis) {
		return
	}
	m.cur, m.sel = i, m.row(i).ID
}

// maxPins is the most ancestors that stay pinned above a scrolled tree.
const maxPins = 2

// scroll moves the window the least that shows the row under the cursor, or
// keeps it where it is inside the row when the row is taller than the window
// (the keys put it at the row's first lines, or its last when [Model.bottom]
// is set), and pins the ancestors
// of the row at its top. The pins take lines off the window and which rows
// they are depends on where it is, so the window is settled again with the
// lines the pins of its last place took, until they take no more than that.
// It starts from the pins the window had, so a cursor that moves within it
// does not shift it for pins that are no longer there, and when the window
// lands where there are fewer, it comes back up as far as the cursor row
// allows.
func (m *Model) scroll() {
	taken, from := len(m.pins), m.off
	defer func() { m.bottom = false }()
	m.pins = nil
	if len(m.vis) == 0 || m.height <= 0 {
		m.off = 0
		return
	}
	s, e := m.starts[m.cur], m.starts[m.cur+1]
	for {
		avail := max(m.height-taken, 1)
		switch {
		case e-s > avail && m.bottom:
			m.off = e - avail
		case e-s > avail:
			// A window inside a row taller than it stays where it is.
			m.off = min(max(m.off, s), e-avail)
		case s < m.off:
			m.off = s
		case e > m.off+avail:
			m.off = e - avail
		}
		m.off = max(min(m.off, m.starts[len(m.vis)]-avail), 0)
		pins := m.pinsAt(m.off)
		if len(pins) <= taken {
			m.pins = pins
			break
		}
		taken = len(pins)
	}
	if len(m.pins) < taken {
		m.pull(s, e, from)
	}
}

// pull moves the window up, a line at a time, while the row that ends at line
// e stays in view and either lines are left blank at the bottom that rows
// above could fill, or the window is still below line from, where it was
// before it moved, or the row that starts at line s fits the window but its
// first lines are above it. The window was settled with more pins than its
// place has, so it went further down than it needed to.
func (m *Model) pull(s, e, from int) {
	total := m.starts[len(m.vis)]
	for m.off > 0 {
		hidden := s < m.off && e-s <= m.height-len(m.pinsAt(m.off))
		if m.off <= total-(m.height-len(m.pinsAt(m.off))) && m.off <= from && !hidden {
			break
		}
		if e > m.off-1+m.height-len(m.pinsAt(m.off-1)) {
			break
		}
		m.off--
	}
	m.pins = m.pinsAt(m.off)
}

// pinsAt returns the places in vis of the ancestors to pin above a window
// that starts at line off, the nearest last: at most [maxPins] of them, and
// always a line left for the rows. A row the cursor may not rest on is not
// pinned.
func (m Model) pinsAt(off int) []int {
	limit := min(maxPins, m.height-1)
	top := sort.Search(len(m.vis), func(i int) bool { return m.starts[i+1] > off })
	if limit <= 0 || top >= len(m.vis) {
		return nil
	}
	var pins []int
	need := m.row(top).Depth
	for j := top - 1; j >= 0 && need > 0; j-- {
		p := m.row(j)
		if p.Depth >= need {
			continue
		}
		need = p.Depth
		if !p.Skip {
			pins = append(pins, j)
		}
	}
	pins = pins[:min(len(pins), limit)]
	slices.Reverse(pins)
	return pins
}

// inner is the number of lines of the window that show rows: what is left
// when the pinned rows have their lines.
func (m Model) inner() int { return m.height - len(m.pins) }

package diff

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// sideStep is how many cells a press of left or right scrolls.
const sideStep = 8

// Update handles keys while focused, and the results of its own fetches. It
// ignores messages meant for other views.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.dirty {
		// SetSize and Seek cannot return a command, so fetch what the new
		// window shows now.
		m.dirty = false
		first := m.sync()
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		return m, tea.Batch(first, cmd)
	}
	switch msg := msg.(type) {
	case highlightMsg:
		if msg.id == m.id && msg.gen == m.hl.gen {
			if msg.busy {
				delete(m.hl.asked, msg.file)
			} else {
				m.hl.spans[msg.file] = msg.spans
			}
		}
		return m, nil
	case pageMsg:
		if msg.id != m.id || !m.pg.fetching || msg.cursor != m.pg.next {
			return m, nil
		}
		cmd := m.receive(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		cmd := m.press(msg)
		return m, cmd
	}
	if m.focused && m.searching {
		// Pastes and the like go to the input, which edits its text in
		// place, so it gets a copy of its own first.
		m.input.SetValue(m.input.Value())
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// receive stores a fetched page.
func (m *Model) receive(msg pageMsg) tea.Cmd {
	m.pg.fetching = false
	if msg.err != nil {
		m.pg.err = msg.err
		m.syncKeys()
		return nil
	}
	m.pg.err = nil
	m.syncKeys()
	m.layout.Append(msg.files...)
	m.searchMore()
	// A source that answers with no files and the cursor it was asked
	// with would be asked again for ever.
	m.pg.done = msg.next == "" || (len(msg.files) == 0 && msg.next == msg.cursor)
	m.pg.next = msg.next
	if m.pg.seek != nil {
		p := *m.pg.seek
		if m.layout.FileIndex(p.pos.Path) >= 0 || m.pg.done {
			m.seek(p.pos, p.file)
		}
	}
	id, files, done := m.id, msg.files, m.pg.done
	tell := func() tea.Msg { return FilesMsg{ID: id, Files: files, Done: done} }
	return tea.Batch(tell, m.sync())
}

// Retry repeats a failed fetch, as the retry key does, such as once the
// network is back. It returns nil if no fetch failed.
func (m *Model) Retry() tea.Cmd {
	if m.pg.err == nil || m.pg.fetching {
		return nil
	}
	m.pg.err, m.pg.fetching = nil, true
	m.syncKeys()
	m.scroll()
	return m.fetchCmd()
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	if m.searching {
		return m.pressSearch(msg)
	}
	m.pg.seek = nil
	m.search.waiting = false
	h := m.bodyHeight()
	switch {
	case key.Matches(msg, m.keyMap.Up):
		m.cursor--
	case key.Matches(msg, m.keyMap.Down):
		m.cursor++
	case key.Matches(msg, m.keyMap.PageUp):
		m.cursor -= h
	case key.Matches(msg, m.keyMap.PageDown):
		m.cursor += h
	case key.Matches(msg, m.keyMap.HalfPageUp):
		m.cursor -= max(h/2, 1)
	case key.Matches(msg, m.keyMap.HalfPageDown):
		m.cursor += max(h/2, 1)
	case key.Matches(msg, m.keyMap.Home):
		m.cursor = 0
	case key.Matches(msg, m.keyMap.End):
		m.cursor = m.layout.Len() - 1
	case key.Matches(msg, m.keyMap.Left):
		m.left = max(m.left-sideStep, 0)
	case key.Matches(msg, m.keyMap.Right):
		m.left = min(m.left+sideStep, m.maxLeft())
	case key.Matches(msg, m.keyMap.NextFile):
		m.nextFile()
	case key.Matches(msg, m.keyMap.PrevFile):
		m.prevFile()
	case key.Matches(msg, m.keyMap.NextHunk):
		m.nextHunk()
	case key.Matches(msg, m.keyMap.PrevHunk):
		m.prevHunk()
	case key.Matches(msg, m.keyMap.Fold):
		m.fold()
	case key.Matches(msg, m.keyMap.Search):
		return m.openSearch()
	case key.Matches(msg, m.keyMap.Next):
		m.step(1)
	case key.Matches(msg, m.keyMap.Prev):
		m.step(-1)
	case m.search.query != "" && key.Matches(msg, m.keyMap.Cancel):
		m.clearSearch()
	case key.Matches(msg, m.keyMap.Retry):
		return m.Retry()
	default:
		return nil
	}
	return m.sync()
}

// fold folds or unfolds the file whose header the cursor is on.
func (m *Model) fold() {
	if f, ok := m.layout.FileAt(m.cursor); ok && m.isHeader(m.cursor) {
		m.layout.SetCollapsed(f, !m.layout.Collapsed(f))
	}
}

// isHeader reports whether row is the header of a file.
func (m Model) isHeader(row int) bool {
	f, ok := m.layout.FileAt(row)
	if !ok {
		return false
	}
	start, _ := m.layout.FileRow(f)
	return start == row
}

func (m *Model) nextFile() {
	f, ok := m.layout.FileAt(m.cursor)
	if !ok || f+1 >= m.layout.Files() {
		return
	}
	m.cursor, _ = m.layout.FileRow(f + 1)
}

func (m *Model) prevFile() {
	f, ok := m.layout.FileAt(m.cursor)
	if !ok {
		return
	}
	if start, _ := m.layout.FileRow(f); m.cursor != start {
		m.cursor = start
	} else if f > 0 {
		m.cursor, _ = m.layout.FileRow(f - 1)
	}
}

// hunkAt reports whether row is a hunk header.
func (m Model) hunkAt(row int) bool {
	r, ok := m.layout.RowAt(row)
	return ok && r.Kind == KindHunkHeader
}

func (m *Model) nextHunk() {
	f, ok := m.layout.FileAt(m.cursor)
	if !ok {
		return
	}
	end, _ := m.layout.FileRow(f + 1)
	for r := m.cursor + 1; r < end; r++ {
		if m.hunkAt(r) {
			m.cursor = r
			return
		}
	}
	// Parsing a file to look for its first hunk costs about as much as
	// drawing it, and a folded file has no hunk rows to go to.
	for g := f + 1; g < m.layout.Files(); g++ {
		if m.layout.Collapsed(g) {
			continue
		}
		if start, _ := m.layout.FileRow(g); m.hunkAt(start + 1) {
			m.cursor = start + 1
			return
		}
	}
}

func (m *Model) prevHunk() {
	f, ok := m.layout.FileAt(m.cursor)
	if !ok {
		return
	}
	for g := f; g >= 0; g-- {
		if m.layout.Collapsed(g) {
			continue
		}
		start, _ := m.layout.FileRow(g)
		from := m.cursor - 1
		if g != f {
			end, _ := m.layout.FileRow(g + 1)
			from = end - 1
		}
		for r := from; r > start; r-- {
			if m.hunkAt(r) {
				m.cursor = r
				return
			}
		}
	}
}

// maxLeft is how far the rows in the window can be scrolled sideways: until
// the widest of their lines ends at the right edge.
func (m Model) maxLeft() int {
	room := m.textWidth(m.numWidth())
	widest := 0
	h := m.bodyHeight()
	for r := m.top; r < min(m.top+h, m.layout.Len()); r++ {
		if row, ok := m.layout.RowAt(r); ok && scrolls(row.Kind) {
			widest = max(widest, ansi.StringWidth(termtext.Clean(row.Text, m.tabs)))
		}
	}
	return max(widest-room, 0)
}

// scrolls reports whether rows of kind move with a sideways scroll: the
// lines, but not the headers and the notes.
func scrolls(k Kind) bool {
	switch k {
	case KindContext, KindAdded, KindDeleted, KindNoNewline, KindRaw:
		return true
	case KindFileHeader, KindHunkHeader, KindNote:
	}
	return false
}

// sync moves the window to the cursor, fetches what the window is about
// to show, and highlights the files in it.
func (m *Model) sync() tea.Cmd {
	m.scroll()
	if !m.wantsMore() {
		return m.highlight()
	}
	m.pg.fetching = true
	// Bring the loading row into view.
	m.scroll()
	return tea.Batch(m.fetchCmd(), m.highlight())
}

// wantsMore reports whether the next page should be fetched: the window is
// within a window's height of the last row.
func (m Model) wantsMore() bool {
	if m.pg.done || m.pg.fetching || m.pg.err != nil {
		return false
	}
	// A search shown covers every file, so the pages go on to be fetched.
	return m.pg.seek != nil || m.search.re != nil || m.top+2*max(m.bodyHeight(), 1) >= m.layout.Len()
}

// bodyHeight is the number of rows of the window: the view without its
// status line, which a view of fewer than three rows leaves out so that the
// loading or error row still has room.
func (m Model) bodyHeight() int {
	if m.height >= 3 {
		return m.height - 1
	}
	return m.height
}

// hasStatus reports whether a loading, error or empty row follows the rows.
func (m Model) hasStatus() bool {
	return m.pg.fetching || m.pg.err != nil || m.layout.Len() == 0
}

// sticky (only in a window of three rows or more, so a small one keeps room
// for the cursor and the loading row) reports whether the header of the file the first row of the
// window is in stands on the window's first row, since that row is inside
// the file and its own header is above the window. The row the window
// starts at is then covered by it.
func (m Model) sticky() bool {
	if m.bodyHeight() < 3 || m.top >= m.layout.Len() {
		return false
	}
	f, ok := m.layout.FileAt(m.top)
	if !ok {
		return false
	}
	start, _ := m.layout.FileRow(f)
	return start != m.top
}

// scroll keeps the cursor in range and in view.
func (m *Model) scroll() {
	h := m.bodyHeight()
	n := m.layout.Len()
	m.cursor = max(min(m.cursor, n-1), 0)
	total := n
	if m.hasStatus() {
		total++
	}
	m.top = max(min(m.top, total-h), 0)
	m.top = min(m.top, m.cursor)
	if m.cursor > m.top+h-1 {
		m.top = m.cursor - h + 1
	}
	// On the last row, show the loading or error row below it too.
	if h >= 2 && m.cursor == n-1 && total > n {
		m.top = max(m.top, total-h)
	}
	// A row in a file at the top of the window is covered by the header.
	if m.cursor == m.top && m.top > 0 && h >= 3 && !m.isHeader(m.top) {
		if _, ok := m.layout.FileAt(m.top); ok {
			m.top--
		}
	}
}

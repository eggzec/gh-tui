package feed

import (
	"errors"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Update handles keys while focused, and the feed's own fetch results and
// spinner ticks. It ignores messages meant for other feeds.
func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd) {
	if m.resized {
		// SetSize cannot return a command, so fetch what a larger window
		// shows now.
		m.resized = false
		sizeCmd := m.sync()
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		return m, tea.Batch(sizeCmd, cmd)
	}
	switch msg := msg.(type) {
	case chunkMsg[T]:
		if msg.id != m.id || msg.gen != m.gen {
			return m, nil
		}
		cmd := m.receive(msg)
		return m, cmd
	case spinner.TickMsg:
		if msg.ID != m.spin.ID() {
			return m, nil
		}
		if !m.tail.fetching {
			m.spinning = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		m.note = ""
		if m.prompt.Focused() {
			return m.updatePrompt(msg)
		}
		cmd := m.press(msg)
		m.scroll()
		return m, cmd
	}
	if m.prompt.Focused() {
		// Pastes and the like go to the prompt.
		return m.updatePrompt(msg)
	}
	return m, nil
}

func (m *Model[T]) press(msg tea.KeyPressMsg) tea.Cmd {
	page := m.slots()
	before := m.sel
	switch {
	case keymap.Matches(msg, m.keyMap.Up):
		m.sel--
	case keymap.Matches(msg, m.keyMap.Down):
		m.sel++
	case keymap.Matches(msg, m.keyMap.PageUp):
		m.sel -= page
	case keymap.Matches(msg, m.keyMap.PageDown):
		m.sel += page
	case keymap.Matches(msg, m.keyMap.HalfPageUp):
		m.sel -= max(page/2, 1)
	case keymap.Matches(msg, m.keyMap.HalfPageDown):
		m.sel += max(page/2, 1)
	case keymap.Matches(msg, m.keyMap.Home):
		m.sel = 0
	case keymap.Matches(msg, m.keyMap.End):
		m.sel = m.shown() - 1
	case keymap.Matches(msg, m.keyMap.Retry):
		return m.Retry()
	case keymap.Matches(msg, m.keyMap.Find):
		return m.openPrompt(promptFind)
	case keymap.Matches(msg, m.keyMap.QuickFilter):
		return m.openPrompt(promptFilter)
	case keymap.Matches(msg, m.keyMap.Next):
		return m.step(1)
	case keymap.Matches(msg, m.keyMap.Prev):
		return m.step(-1)
	case keymap.Matches(msg, m.markKeys.Mark) && m.canMark():
		m.toggleMark()
		return nil
	case keymap.Matches(msg, m.promptKeys.Cancel):
		// Esc peels one layer at a time: the find, then the filter, then
		// the marks. The prompt, which comes before them all, takes its
		// own cancel key while it is open.
		cmd, _ := m.ClearTransient()
		return cmd
	default:
		return nil
	}
	// The user moved, so a Reload no longer needs to follow the item.
	m.anchored = false
	// A filter does not read ahead, since rows it hides would have it
	// fetch every chunk: moving down from its last row, or from where it
	// shows none, fetches the next one.
	n := m.shown()
	past := m.filter != "" && (n == 0 || before == n-1) && m.sel > before && m.sel >= n
	cmd := m.sync()
	if past {
		cmd = tea.Batch(cmd, m.fetchMore())
	}
	return cmd
}

// fetchMore fetches the next chunk, if there is one and it is not being
// fetched.
func (m *Model[T]) fetchMore() tea.Cmd {
	if m.done || m.tail.fetching || m.tail.err != nil {
		return nil
	}
	return m.startFetch(len(m.chunks))
}

// Retry repeats every failed fetch, as the retry key does, such as once
// the network is back. It returns nil if no fetch failed.
func (m *Model[T]) Retry() tea.Cmd {
	var cmd tea.Cmd
	for i := range len(m.chunks) + 1 {
		if m.chunk(i).err != nil {
			cmd = tea.Batch(cmd, m.startFetch(i))
		}
	}
	return cmd
}

// RetryKept fetches again every loaded chunk whose items came with
// [ErrKept], such as once the source can give new ones, and returns nil
// if none did. A chunk being fetched already is left to finish.
func (m *Model[T]) RetryKept() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.chunks {
		if c := &m.chunks[i]; c.kept && c.loaded && !c.fetching {
			if cmd == nil {
				// The new items may move the selected one, as after a
				// Reload.
				m.anchorSelection()
			}
			cmd = tea.Batch(cmd, m.startFetch(i))
		}
	}
	return cmd
}

// receive stores a fetched chunk. Results that no longer match a chunk, for
// example because the chunk was already fetched again, are dropped.
func (m *Model[T]) receive(msg chunkMsg[T]) tea.Cmd {
	if msg.index > len(m.chunks) {
		return nil
	}
	c := m.chunk(msg.index)
	if c.cursor != msg.cursor || !c.fetching {
		return nil
	}
	c.fetching = false
	stale, kept := errors.Is(msg.err, ErrStale), errors.Is(msg.err, ErrKept)
	if msg.err != nil && !stale && !kept {
		c.err, c.kept = msg.err, false
		m.refreshError()
		return nil
	}

	appended := msg.index == len(m.chunks)
	if appended {
		m.chunks = append(m.chunks, chunk[T]{cursor: msg.cursor})
	}
	c = &m.chunks[msg.index]
	c.items, c.n, c.loaded, c.kept, c.texts = msg.items, len(msg.items), true, kept, nil
	m.pages++
	if appended || msg.next != c.next {
		// The chunks after this one no longer follow from it, so fetch
		// them again from its new next cursor.
		clear(m.chunks[msg.index+1:])
		m.chunks = m.chunks[:msg.index+1]
		c.next = msg.next
		m.tail = chunk[T]{cursor: msg.next}
		m.done = msg.next == ""
		m.refreshError()
	}
	m.reindex()
	m.pruneMarks()
	if m.anchored {
		if i, ok := m.indexOf(m.anchor); ok {
			i = m.posOf(i)
			m.sel, m.top = i, i-m.anchorRow
		}
	}
	cmd := m.sync()
	if !stale {
		return cmd
	}
	m.anchorSelection()
	return tea.Batch(cmd, m.startFetch(msg.index))
}

// sync moves the window to the selection, fetches what the window is about
// to show, and evicts what it no longer needs.
func (m *Model[T]) sync() tea.Cmd {
	m.scroll()
	cmd := m.keep()
	if !m.wantsTail() {
		return cmd
	}
	cmd = tea.Batch(cmd, m.startFetch(len(m.chunks)))
	// Bring the loading row into view.
	m.scroll()
	return cmd
}

// wantsTail reports whether the next chunk should be fetched: the window is
// not full, or the selection is within the prefetch threshold of the end.
func (m Model[T]) wantsTail() bool {
	if m.done || m.tail.fetching || m.tail.err != nil || m.filter != "" {
		return false
	}
	return m.top+m.slots() >= m.total || m.sel+m.margin() >= m.total-1
}

// margin returns how many rows beyond the window the feed keeps loaded.
func (m Model[T]) margin() int {
	if m.prefetch > 0 {
		return m.prefetch
	}
	return m.slots()
}

// keep fetches the evicted chunks near the window again, and evicts the
// chunks farthest from it while more than maxChunks are loaded. Positions
// stay stable because evicted chunks keep their cursor and length.
func (m *Model[T]) keep() tea.Cmd {
	// A filter shows rows by position among the items it keeps, so the
	// window says nothing of which chunks are near it.
	if m.total == 0 || m.filter != "" {
		return nil
	}
	margin := m.margin()
	lo := m.chunkAt(max(m.top-margin, 0))
	hi := m.chunkAt(min(m.top+m.slots()+margin, m.total) - 1)

	var cmd tea.Cmd
	loaded := 0
	for i := range m.chunks {
		c := &m.chunks[i]
		if c.loaded {
			loaded++
			continue
		}
		if i >= lo && i <= hi && !c.fetching && c.err == nil {
			cmd = tea.Batch(cmd, m.startFetch(i))
		}
	}
	for loaded > m.maxChunks {
		far, dist := -1, 0
		for i, c := range m.chunks {
			if d := max(lo-i, i-hi); c.loaded && d > dist {
				far, dist = i, d
			}
		}
		if far < 0 {
			break
		}
		m.chunks[far].items, m.chunks[far].loaded, m.chunks[far].texts = nil, false, nil
		loaded--
	}
	return cmd
}

// scroll keeps the selection in range and in view.
func (m *Model[T]) scroll() {
	n := m.shown()
	m.sel = max(min(m.sel, n-1), 0)

	slots := m.slots()
	rows := n
	if m.hasStatus() {
		rows++
	}
	// Fill the window when it grows, rather than leaving space at the end.
	m.top = max(min(m.top, rows-slots), 0)
	m.top = min(m.top, m.sel)
	if m.sel >= m.top+slots {
		m.top = m.sel - slots + 1
	}
	// On the last item, show the loading or error row below it too.
	if m.sel == n-1 && rows > n {
		m.top = max(m.top, rows-slots)
	}
}

// slots returns the number of rows that fit in the window, at least one.
func (m Model[T]) slots() int {
	h := m.height
	if m.hasFooter() {
		h--
	}
	return max(h/m.itemHeight, 1)
}

// hasStatus reports whether a loading, error or empty row follows the items.
func (m Model[T]) hasStatus() bool {
	return m.tail.fetching || m.tail.err != nil || m.shown() == 0
}

// ClearTransient peels one layer of what the user asked to see: the find
// shown, else the filter, else the marks. It reports whether there was
// one. The parent calls it for the key that dismisses, which clears these
// before it closes anything.
func (m *Model[T]) ClearTransient() (tea.Cmd, bool) {
	switch {
	case m.query != "":
		m.clearFind()
		return nil, true
	case m.filter != "":
		return m.clearFilter(), true
	case len(m.marks) > 0:
		m.ClearMarks()
		return nil, true
	}
	return nil, false
}

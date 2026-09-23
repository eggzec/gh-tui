package feed

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys while focused, and the feed's own fetch results and
// spinner ticks. It ignores messages meant for other feeds.
func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd) {
	switch msg := msg.(type) {
	case chunkMsg[T]:
		if msg.id != m.id {
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
		cmd := m.press(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model[T]) press(msg tea.KeyPressMsg) tea.Cmd {
	page := m.slots()
	switch {
	case key.Matches(msg, m.keyMap.Up):
		m.sel--
	case key.Matches(msg, m.keyMap.Down):
		m.sel++
	case key.Matches(msg, m.keyMap.PageUp):
		m.sel -= page
	case key.Matches(msg, m.keyMap.PageDown):
		m.sel += page
	case key.Matches(msg, m.keyMap.Home):
		m.sel = 0
	case key.Matches(msg, m.keyMap.End):
		m.sel = m.total - 1
	case key.Matches(msg, m.keyMap.Retry):
		return m.retry()
	default:
		return nil
	}
	return m.sync()
}

// retry repeats every failed fetch.
func (m *Model[T]) retry() tea.Cmd {
	if m.tail.err == nil {
		return nil
	}
	return m.startFetch(len(m.chunks))
}

// receive stores a fetched chunk. Results that no longer match a chunk, for
// example because the chunk was already fetched again, are dropped.
func (m *Model[T]) receive(msg chunkMsg[T]) tea.Cmd {
	if msg.index != len(m.chunks) || msg.cursor != m.tail.cursor || !m.tail.fetching {
		return nil
	}
	m.tail.fetching = false
	if msg.err != nil {
		m.tail.err = msg.err
		m.refreshError()
		return nil
	}
	m.chunks = append(m.chunks, chunk[T]{
		cursor: msg.cursor,
		next:   msg.next,
		items:  msg.items,
		n:      len(msg.items),
	})
	m.tail = chunk[T]{cursor: msg.next}
	m.done = msg.next == ""
	m.reindex()
	return m.sync()
}

// sync moves the window to the selection and fetches what it needs next.
func (m *Model[T]) sync() tea.Cmd {
	m.scroll()
	if !m.wantsTail() {
		return nil
	}
	cmd := m.startFetch(len(m.chunks))
	// Bring the loading row into view.
	m.scroll()
	return cmd
}

// wantsTail reports whether the next chunk should be fetched: the window is
// not full, or the selection reached the last loaded item.
func (m Model[T]) wantsTail() bool {
	if m.done || m.tail.fetching || m.tail.err != nil {
		return false
	}
	return m.top+m.slots() >= m.total || m.sel >= m.total-1
}

// scroll keeps the selection in range and in view.
func (m *Model[T]) scroll() {
	m.sel = max(min(m.sel, m.total-1), 0)

	slots := m.slots()
	rows := m.total
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
	if m.sel == m.total-1 && rows > m.total {
		m.top = max(m.top, rows-slots)
	}
}

// slots returns the number of rows that fit in the window, at least one.
func (m Model[T]) slots() int {
	return max(m.height/m.itemHeight, 1)
}

// hasStatus reports whether a loading, error or empty row follows the items.
func (m Model[T]) hasStatus() bool {
	return m.tail.fetching || m.tail.err != nil || m.total == 0
}

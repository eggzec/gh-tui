package thread

import (
	"math"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys when focused, mouse wheel scrolling, the spinner and
// fetched chunks. After every message it loads the next chunk if the
// screen is near the end, so a resize that reveals the end loads more on the
// next message.
func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.focused {
			cmd = m.handleKey(msg)
		}
	case tea.MouseWheelMsg:
		if m.focused {
			m.vp, _ = m.vp.Update(msg)
		}
	case spinner.TickMsg:
		// Dropping the tick while nothing loads stops the spinner; the next
		// load starts it again.
		if msg.ID != m.spin.ID() || !m.spinning() {
			return m, nil
		}
		m.spin, cmd = m.spin.Update(msg)
		if m.statusIdx >= 0 {
			m.status = m.statusText()
		}
		return m, cmd
	case fetchedMsg[T]:
		if msg.id != m.id || msg.gen != m.gen {
			return m, nil
		}
		m.apply(msg)
	}
	if more := m.manage(); more != nil {
		return m, tea.Batch(cmd, more)
	}
	return m, cmd
}

func (m *Model[T]) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := &m.keys
	switch {
	case key.Matches(msg, k.Down):
		m.vp.ScrollDown(1)
	case key.Matches(msg, k.Up):
		m.vp.ScrollUp(1)
	case key.Matches(msg, k.PageDown):
		m.vp.PageDown()
	case key.Matches(msg, k.PageUp):
		m.vp.PageUp()
	case key.Matches(msg, k.HalfPageDown):
		m.vp.HalfPageDown()
	case key.Matches(msg, k.HalfPageUp):
		m.vp.HalfPageUp()
	case key.Matches(msg, k.Top):
		m.vp.SetYOffset(0)
	case key.Matches(msg, k.Bottom):
		m.vp.SetYOffset(math.MaxInt)
	case key.Matches(msg, k.Retry):
		return m.retry()
	}
	return nil
}

func (m *Model[T]) spinning() bool {
	return !m.hasDoc || m.tail.loading
}

// manage loads the first chunk once the document is set, and the next one
// when the screen is within a screen of the end.
func (m *Model[T]) manage() tea.Cmd {
	if !m.hasDoc || m.tail.loading || m.tail.err != nil || !m.more() {
		return nil
	}
	if m.started && m.vp.YOffset()+2*m.height < len(m.lines) {
		return nil
	}
	return m.loadTail()
}

// more reports whether chunks remain after the last one.
func (m *Model[T]) more() bool {
	return !m.started || (len(m.chunks) > 0 && m.chunks[len(m.chunks)-1].next != "")
}

func (m *Model[T]) loadTail() tea.Cmd {
	var cursor string
	if n := len(m.chunks); n > 0 {
		cursor = m.chunks[n-1].next
	}
	a := m.anchor()
	m.started = true
	m.tail.loading, m.tail.err = true, nil
	m.tail.seq++
	m.layout(a)
	return tea.Batch(m.fetchCmd(len(m.chunks), m.tail.seq, true, cursor), m.spin.Tick)
}

func (m *Model[T]) retry() tea.Cmd {
	if m.tail.err != nil {
		return m.loadTail()
	}
	return nil
}

func (m *Model[T]) apply(msg fetchedMsg[T]) {
	if !msg.tail || !m.tail.loading || msg.seq != m.tail.seq || msg.index != len(m.chunks) {
		return
	}
	a := m.anchor()
	m.tail.loading = false
	if msg.err != nil {
		m.tail.err = msg.err
	} else {
		c := chunk[T]{cursor: msg.cursor, next: msg.next, items: msg.items}
		m.renderChunk(&c)
		m.chunks = append(m.chunks, c)
	}
	m.layout(a)
}

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
	case key.Matches(msg, k.Toggle):
		m.toggle()
	}
	return nil
}

func (m *Model[T]) spinning() bool {
	return !m.hasDoc || m.tail.loading
}

// manage keeps the chunks around the screen in memory. It loads the first
// chunk once the document is set and the next one within a screen of the
// end, fetches evicted chunks again as the screen nears them, and evicts
// chunks far from it beyond maxChunks.
func (m *Model[T]) manage() tea.Cmd {
	if !m.hasDoc {
		return nil
	}
	y := m.vp.YOffset()
	lo, hi := y-m.height, y+2*m.height

	var cmd tea.Cmd
	relayout := false
	for i := range m.chunks {
		c := &m.chunks[i]
		near := m.overlaps(i, lo, hi)
		switch {
		case c.loaded || c.loading:
		case c.err != nil && !near:
			// Out of sight, a failed chunk is fetched again on its own
			// when the screen comes back to it.
			c.err, relayout = nil, true
		case c.err == nil && near:
			cmd = batch(cmd, m.loadChunk(i))
		}
	}
	if m.evict(lo, hi) || relayout {
		m.layout(m.anchor())
	}
	if m.tail.loading || m.tail.err != nil || !m.more() {
		return cmd
	}
	if m.started && hi < len(m.lines) {
		return cmd
	}
	return batch(cmd, m.loadTail())
}

// batch is tea.Batch for two commands, without allocating when one is nil,
// which is the common case on a scroll key.
func batch(a, b tea.Cmd) tea.Cmd {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	return tea.Batch(a, b)
}

func (m *Model[T]) overlaps(i, lo, hi int) bool {
	start := m.starts[i]
	return start < hi && start+m.chunks[i].height > lo
}

// evict drops the loaded chunks farthest from lines [lo, hi) until at most
// maxChunks remain, and reports whether it dropped any. Chunks within
// [lo, hi) always stay.
func (m *Model[T]) evict(lo, hi int) bool {
	if m.maxChunks <= 0 {
		return false
	}
	n := 0
	for i := range m.chunks {
		if m.chunks[i].loaded && m.chunks[i].height > 0 {
			n++
		}
	}
	evicted := false
	for ; n > m.maxChunks; n-- {
		far, dist := -1, 0
		for i := range m.chunks {
			c := &m.chunks[i]
			if !c.loaded || c.height == 0 || m.overlaps(i, lo, hi) {
				continue
			}
			d := max(lo-(m.starts[i]+c.height), m.starts[i]-hi)
			if far < 0 || d > dist {
				far, dist = i, d
			}
		}
		if far < 0 {
			break
		}
		c := &m.chunks[far]
		c.items, c.lines, c.starts, c.heads, c.loaded = nil, nil, nil, nil, false
		evicted = true
	}
	return evicted
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

// loadChunk fetches chunk i again.
func (m *Model[T]) loadChunk(i int) tea.Cmd {
	c := &m.chunks[i]
	c.loading, c.err = true, nil
	c.seq++
	return m.fetchCmd(i, c.seq, false, c.cursor)
}

// retry fetches again whatever failed: the next chunk or evicted ones.
func (m *Model[T]) retry() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.chunks {
		if m.chunks[i].err != nil {
			cmd = batch(cmd, m.loadChunk(i))
		}
	}
	if cmd != nil {
		m.layout(m.anchor())
	}
	if m.tail.err != nil {
		cmd = batch(cmd, m.loadTail())
	}
	return cmd
}

func (m *Model[T]) apply(msg fetchedMsg[T]) {
	if msg.tail {
		m.applyTail(msg)
		return
	}
	if msg.index >= len(m.chunks) {
		return
	}
	c := &m.chunks[msg.index]
	if !c.loading || msg.seq != c.seq {
		return
	}
	a := m.anchor()
	c.loading = false
	if msg.err != nil {
		c.err = msg.err
	} else {
		c.items = msg.items
		if msg.index == len(m.chunks)-1 {
			// Only the last chunk's next matters: it may have gained one.
			c.next = msg.next
		}
		m.renderChunk(c)
	}
	m.layout(a)
}

func (m *Model[T]) applyTail(msg fetchedMsg[T]) {
	if !m.tail.loading || msg.seq != m.tail.seq || msg.index != len(m.chunks) {
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

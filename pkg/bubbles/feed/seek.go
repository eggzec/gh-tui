package feed

import tea "charm.land/bubbletea/v2"

// Seek selects the item that [WithKey] gives key as soon as it is loaded,
// and fetches the next chunk, again and again, until it is or the last chunk
// or a failed fetch ends the search; then the selection stays where it was.
// It does nothing without a key function. Call it before Init to seek from
// the first chunk; Init and the fetches it starts do the rest, so the
// command it returns is nil then.
func (m *Model[T]) Seek(key string) tea.Cmd {
	if m.key == nil || key == "" {
		return nil
	}
	m.seek, m.seeking = key, true
	if len(m.chunks) == 0 {
		return nil
	}
	cmd := m.seekStep()
	return tea.Batch(cmd, m.sync())
}

// Seeking reports whether a [Model.Seek] is still looking for its item, so
// that a parent can wait for the selection to settle before it follows it.
func (m Model[T]) Seeking() bool {
	return m.seeking
}

// SelectChild puts the cursor on child row k of the item that [WithKey] gives
// key, and opens the item if it is closed. It reports false, and moves
// nothing, unless that item is loaded, shown, and has more than k children.
func (m *Model[T]) SelectChild(key string, k int) bool {
	if m.key == nil || m.kids == nil || k < 0 {
		return false
	}
	i, ok := m.indexOf(key)
	if !ok {
		return false
	}
	at := -1
	for j, l := range m.lines {
		if !l.head && !l.more && !l.child && l.item == i {
			at = j
			break
		}
	}
	if at < 0 || m.lines[at].kids <= k {
		return false
	}
	if at+1 >= len(m.lines) || !m.lines[at+1].child {
		m.sel = at
		// Opening adds lines only; a chunk is fetched later, if the window
		// needs one.
		m.setOpen(true)
	}
	ref := m.itemRef(i)
	ref.child, ref.k = true, k
	m.restore(ref)
	if !m.onChild() {
		return false
	}
	m.anchored = false
	m.scroll()
	return true
}

// seekStep selects the item sought if it is loaded, and else asks for the
// chunk after the last one, unless there is none or it can't be fetched.
func (m *Model[T]) seekStep() tea.Cmd {
	if i, ok := m.indexOf(m.seek); ok {
		m.seeking, m.anchored = false, false
		m.sel = max(m.posOf(i), 0)
		m.scroll()
		return nil
	}
	if m.done || m.tail.err != nil {
		m.seeking = false
		return nil
	}
	if m.tail.fetching {
		// receive calls seekStep again when that fetch arrives.
		return nil
	}
	return m.startFetch(len(m.chunks))
}

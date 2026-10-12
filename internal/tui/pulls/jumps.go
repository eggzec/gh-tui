package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// The jumps between the review threads of the modal: the keys for the next and
// the previous thread step through them on the Files and Conversation tabs,
// wrapping at the ends, and take any other tab to the diff. A jump from the
// Conversation tab to the diff, such as the one that shows a thread there,
// leaves a return point, which backspace goes back to.

// returnPoint is the place a jump to the diff left: the tab it left, and on
// the Conversation tab the id of the row that had the cursor.
type returnPoint struct {
	tab modalTab
	row string
}

// pushReturn remembers the tab shown, and the row of the Conversation tab
// under the cursor, to come back to.
func (m *detailModal) pushReturn() {
	r := returnPoint{tab: m.tab}
	if m.onConversation() && m.th.rowCount() > 0 {
		r.row = m.th.rowIDAt(m.th.cursorRow())
	}
	if n := len(m.back); n > 0 && m.back[n-1] == r {
		return
	}
	m.back = append(m.back, r)
}

// popReturn goes back to the place the last jump left, and reports whether
// there was one. A return point on the tab that shows is dropped, since
// coming back to it shows nothing new.
func (m *detailModal) popReturn() (tea.Cmd, bool) {
	for len(m.back) > 0 {
		r := m.back[len(m.back)-1]
		m.back = m.back[:len(m.back)-1]
		if r.tab == m.tab || !m.has(r.tab) {
			continue
		}
		if r.row != "" {
			t := &m.th
			for i := range t.rowCount() {
				if t.rowIDAt(i) == r.row {
					t.moveTo(i)
					break
				}
			}
		}
		cmd := m.switchTo(r.tab)
		if r.tab == conversationTab {
			m.revealRow(m.th.cursorRow())
		}
		return cmd, true
	}
	return nil, false
}

// jumpThread moves to the next thread, or to the previous one if d is -1. The
// Files and Conversation tabs step through their threads, and from another tab
// the next thread is the first and the previous the last, in the diff.
func (m *detailModal) jumpThread(d int) tea.Cmd {
	if !m.readableThreads() {
		return nil
	}
	switch {
	case m.onFiles():
		return m.jumpDiff(d, false)
	case m.onConversation():
		return m.jumpConversation(d)
	}
	t := &m.th
	if !t.started || t.dirty || t.err != nil {
		// Once the threads are read, the jump goes on in receiveThreads.
		t.jumping = d
		return m.readThreads()
	}
	return m.enterDiff(d)
}

// enterDiff shows the Files tab on the first thread, or the last if d is -1,
// and leaves a return point.
func (m *detailModal) enterDiff(d int) tea.Cmd {
	if len(m.fileThreads()) == 0 {
		return ui.Notify(toast.Info, "No review threads.")
	}
	m.pushReturn()
	return tea.Batch(m.switchTo(filesTab), m.jumpDiff(d, true))
}

// fileThreads returns the threads that have a file, which are those the diff
// shows.
func (m *detailModal) fileThreads() []*core.Thread {
	var out []*core.Thread
	for i := range m.th.all {
		if m.th.all[i].Path != "" {
			out = append(out, &m.th.all[i])
		}
	}
	return out
}

// jumpConversation moves the cursor to the next unresolved thread, or the
// previous one, wrapping at the ends.
func (m *detailModal) jumpConversation(d int) tea.Cmd {
	t := &m.th
	n := len(t.list)
	if n == 0 || !t.threadsShow() {
		return ui.Notify(toast.Info, "No unresolved threads.")
	}
	next := t.cursorRow() + d
	var note tea.Cmd
	switch {
	case next < 0:
		next = n - 1
		note = ui.Notify(toast.Info, "Wrapped to the last thread.")
	case next >= n:
		next = 0
		note = ui.Notify(toast.Info, "Wrapped to the first thread.")
	}
	t.moveTo(next)
	cmd := m.show()
	m.revealRow(next)
	return tea.Batch(cmd, note)
}

// revealRow scrolls the Conversation tab so that row i of the threads shows,
// with the document's head when it is the first.
func (m *detailModal) revealRow(i int) {
	rows := m.th.rows
	if i < 0 || i >= len(rows) {
		return
	}
	top, h := m.thread.YOffset(), m.thread.Height()
	switch r := rows[i]; {
	case i == 0 && r+2 > h:
		m.thread.SetYOffset(0)
	case r < top:
		m.thread.SetYOffset(r)
	case r+2 > top+h:
		// A row spans up to two lines.
		m.thread.SetYOffset(r + 2 - h)
	}
}

// jumpDiff moves the diff's cursor to the next thread, or the previous one if
// d is -1, and wraps round at the ends. With enter set, it starts from the
// first thread, or the last if d is -1, which is how a jump from another tab
// arrives.
func (m *detailModal) jumpDiff(d int, enter bool) tea.Cmd {
	threads := m.fileThreads()
	if len(threads) == 0 {
		return ui.Notify(toast.Info, "No review threads.")
	}
	last := len(threads) - 1
	pick := func(ts []*core.Thread) *core.Thread {
		if d < 0 {
			return ts[len(ts)-1]
		}
		return ts[0]
	}
	f := m.files
	if f == nil || f.set.len() == 0 {
		// The files aren't read yet: the diff finds the thread as they arrive.
		return m.showThreadAt(pick(threads))
	}

	// later are the threads of files that the diff has not fetched yet, which
	// come after all those it has.
	var later []*core.Thread
	if !f.diff.Done() {
		for _, th := range threads {
			if _, ok := f.set.index(th.Path); !ok {
				later = append(later, th)
			}
		}
	}
	anyBlock := func(diff.Block) bool { return true }
	step := f.diff.NextBlock
	if d < 0 {
		step = f.diff.PrevBlock
	}
	var note tea.Cmd
	wrapped := func() {
		if enter {
			return
		}
		if d < 0 {
			note = ui.Notify(toast.Info, "Wrapped to the last thread.")
			return
		}
		note = ui.Notify(toast.Info, "Wrapped to the first thread.")
	}
	moved := !enter && step(anyBlock)
	if !moved {
		switch {
		case d > 0 && !enter && len(later) > 0:
			return m.showThreadAt(later[0])
		case d < 0 && len(later) > 0:
			wrapped()
			return tea.Batch(m.showThreadAt(later[len(later)-1]), note)
		}
		// Go round: to the first block, or to the last by way of all of them.
		wrapped()
		f.diff.SeekFile(f.set.list()[0].Path)
		moved = f.diff.NextBlock(anyBlock)
		if d < 0 {
			for range last {
				if !f.diff.NextBlock(anyBlock) {
					break
				}
			}
		}
		if !moved {
			if len(later) > 0 {
				return m.showThreadAt(later[0])
			}
			return ui.Notify(toast.Info, "No review threads.")
		}
	}
	f.setFocus(diffPane)
	m.layoutFiles()
	var cmd tea.Cmd
	// Fetches what the move shows, which the diff does on its next update.
	f.diff, cmd = f.diff.Update(nil)
	return tea.Batch(cmd, m.follow(), m.spinWhileLoading(), note)
}

// finishJump goes on with a jump that waited for the first page of the
// threads, which has come in, and drops it if the tab changed or the read
// failed.
func (m *detailModal) finishJump(msg threadsMsg) tea.Cmd {
	t := &m.th
	d := t.jumping
	if d == 0 || msg.more {
		return nil
	}
	t.jumping = 0
	if msg.err != nil || m.showsThreads() {
		return nil
	}
	return m.enterDiff(d)
}

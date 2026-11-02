package conv

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// Selection is the row under the cursor, with the data it shows. A thread row
// holds its thread and first comment, and a reply its thread and comment.
type Selection struct {
	ID, Kind string
	Item     *core.TimelineItem
	Thread   *core.Thread
	Comment  *core.ThreadComment
}

// Current returns the row under the cursor with its data.
func (m Model) Current() (Selection, bool) {
	v := m.views[m.cur]
	if v == nil {
		return Selection{}, false
	}
	sel, ok := v.tree.Selected()
	if !ok {
		return Selection{}, false
	}
	id := sel.ID
	n := v.nodes[id]
	if n == nil {
		return Selection{}, false
	}
	return Selection{ID: id, Kind: n.kind, Item: n.item, Thread: n.thread, Comment: n.comment}, true
}

// Recache reads the kept pages again, with no read from the network, so that
// a change that the service made to its cache shows, or a rollback does.
func (m Model) Recache() (Model, tea.Cmd) {
	return m, tea.Batch(m.localCmd(timeline), m.localCmd(threads))
}

// localCmd is cachedCmd for a read that only takes in what the cache holds.
func (m Model) localCmd(src which) tea.Cmd {
	read := m.cachedCmd(src)
	return func() tea.Msg {
		msg := read().(loadMsg)
		msg.local = true
		return msg
	}
}

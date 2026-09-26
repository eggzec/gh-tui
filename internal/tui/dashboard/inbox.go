package dashboard

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// inboxList is the notifications pane: the unread threads of the first
// page of the inbox, with a cursor, which opens what a thread is about as
// the notifications screen does.
type inboxList struct {
	rows []core.Notification
	sel  int
	// top is the first row on view.
	top int
}

// set lists the unread threads of p, and keeps the cursor on the thread
// it was on if it is still listed.
func (l *inboxList) set(p core.Page[core.Notification]) {
	prev, had := l.selected()
	rows := make([]core.Notification, 0, len(p.Items))
	for i := range p.Items {
		if p.Items[i].Unread {
			rows = append(rows, p.Items[i])
		}
	}
	l.rows = rows
	if had {
		if i := slices.IndexFunc(rows, func(n core.Notification) bool { return n.ID == prev.ID }); i >= 0 {
			l.sel = i
		}
	}
	l.sel = max(min(l.sel, len(rows)-1), 0)
}

// selected returns the thread under the cursor.
func (l *inboxList) selected() (core.Notification, bool) {
	return l.item(l.sel)
}

// item returns the thread at index i.
func (l *inboxList) item(i int) (core.Notification, bool) {
	if i < 0 || i >= len(l.rows) {
		return core.Notification{}, false
	}
	return l.rows[i], true
}

// move moves the cursor by d rows, within the list.
func (l *inboxList) move(d int) {
	l.sel = max(min(l.sel+d, len(l.rows)-1), 0)
}

// scroll keeps the cursor within the h rows on view.
func (l *inboxList) scroll(h int) {
	h = max(h, 1)
	l.top = max(min(l.top, l.sel), l.sel-h+1, 0)
}

// setInbox shows the inbox that notes holds.
func (s *Section) setInbox() {
	s.threads.set(s.notes.value)
}

// openThread opens what the thread under the cursor is about, and marks it
// read if opening marks threads read.
func (s *Section) openThread() tea.Cmd {
	n, ok := s.threads.selected()
	if !ok {
		return nil
	}
	cmd := s.opener.Open(n)
	if s.opener.MarksRead() && n.Unread && s.marker != nil {
		op := s.marker.MarkRead(n.ID)
		// The mark shows at once in the cache the pane reads.
		s.readInboxCache()
		cmd = tea.Batch(cmd, ui.Do(s.ctx, ui.NotificationsTitle, op, "mark read"))
	}
	return cmd
}

// readAhead reads ahead what the first unread threads are about while the
// dashboard is on view, and the one under the cursor while the pane has the
// focus.
func (s *Section) readAhead() tea.Cmd {
	if !s.started || s.inbox == nil || !s.focused {
		return nil
	}
	n, ok := s.threads.selected()
	return s.opener.ReadAhead(s.threads.item, n, ok && s.focus == inboxPane)
}

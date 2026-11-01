package dashboard

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	tuinotes "github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// inboxList is the notifications pane: the unread threads of the first
// page of the inbox, with a cursor, which opens what a thread is about as
// the notifications list does.
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
	if s.reader != nil {
		s.states = s.reader.CachedStates(s.threads.rows)
		s.wantStates = true
	}
}

// statesMsg carries the states a read of the subjects of the threads
// answered.
type statesMsg struct {
	states map[core.SubjectRef]core.SubjectState
}

// readStates reads the state of what the threads are about, once they
// changed, which the rows show.
func (s *Section) readStates() tea.Cmd {
	if !s.wantStates || s.reader == nil {
		return nil
	}
	s.wantStates = false
	if len(s.threads.rows) == 0 {
		return nil
	}
	rows, reader, ctx := slices.Clone(s.threads.rows), s.reader, s.ctx
	return func() tea.Msg {
		states, err := reader.States(ctx, rows)
		if err != nil || len(states) == 0 {
			return nil
		}
		return statesMsg{states}
	}
}

// subjectState is the state of what thread n is about, if it is known.
func (s *Section) subjectState(n *core.Notification) (core.SubjectState, bool) {
	st, ok := s.states[core.SubjectRef{Repo: n.Repo, Number: n.Subject.Number, Type: n.Subject.Type}]
	return st, ok
}

// markThread asks whether to mark the thread under the cursor read, or done,
// with the keys and the question the notifications list has.
func (s *Section) markThread(done bool) tea.Cmd {
	if s.marker == nil {
		return nil
	}
	if cmd, refused := (ui.Gate{Token: s.voice.Token, Icons: s.icons}).Refuse(ui.ActMarkRead, nil); refused {
		return cmd
	}
	run := func(n core.Notification) tea.Cmd {
		op, what := s.marker.MarkRead(n.ID), "mark read"
		if done {
			op, what = s.marker.MarkDone(n.ID), "mark done"
		}
		// The change shows at once in the cache the pane reads.
		s.readInboxCache()
		return ui.Do(s.ctx, ui.NotificationsTitle, op, what)
	}
	return tuinotes.ConfirmThread(s.threads.selected, done, run, s.keys.confirm, s.icons)
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

// readAhead reads ahead what the unread threads around the cursor are
// about while the dashboard is on view.
func (s *Section) readAhead() tea.Cmd {
	if !s.started || !s.focused {
		return nil
	}
	s.opener.FollowInbox(true)
	if s.inbox == nil || !s.notes.ok {
		// There is no inbox, or it hasn't loaded.
		return s.opener.ReadAhead(nil, 0)
	}
	return s.opener.ReadAhead(s.threads.item, s.threads.sel)
}

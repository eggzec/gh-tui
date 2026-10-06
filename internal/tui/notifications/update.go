package notifications

import (
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// Update handles the section's keys, sync events and finished changes, and
// passes everything else to the list.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(ui.AheadMsg); ok {
		// The opener may be the dashboard's too, which reads ahead while
		// it is on view instead.
		if !s.feed.Focused() {
			return nil
		}
		return s.opener.Rested(msg)
	}
	cmd := s.update(msg)
	if ahead := s.readAhead(); ahead != nil {
		cmd = tea.Batch(cmd, ahead)
	}
	return cmd
}

func (s *Section) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if cmd, ok := s.press(msg); ok {
			return cmd
		}
	case ui.SyncMsg:
		// Poll already refreshed the cache, so a reload reads it. A failed
		// poll changed nothing.
		if msg.Key != SyncKey || msg.Err != nil {
			return nil
		}
		return s.reload()
	case ui.OnlineMsg:
		// A rate limit is the token's, and has lifted unless one holds.
		if !msg.Limited {
			s.opener.Resume()
		}
		return ui.RetryUnreached(&s.feed)
	case ui.SettingsMsg:
		s.configure(msg.Config)
		return nil
	case ui.DoneMsg:
		// A failed change was rolled back in the cache; a successful one
		// was applied again.
		if msg.From != ui.NotificationsTitle {
			return nil
		}
		return s.reload()
	case ui.AccessMsg:
		// What the token was refused, or failed to read, it may read
		// now. While it still may not, a reload would send nothing and
		// only show the list loading.
		blocked := s.unreadable != ""
		s.renderUnreadable()
		if s.refused() || !blocked && s.feed.Err() == nil {
			return nil
		}
		return s.reload()
	}
	var cmd tea.Cmd
	s.feed, cmd = s.feed.Update(msg)
	return cmd
}

// press handles the section's own keys and reports whether msg was one.
func (s *Section) press(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if !s.feed.Focused() {
		return nil, false
	}
	k := &s.keys
	switch {
	case key.Matches(msg, k.Refresh):
		s.svc.Invalidate()
		s.opener.Resume()
		return s.reload(), true
	case key.Matches(msg, k.ClearFilter):
		if !s.filtered() {
			return nil, true
		}
		return s.setFilter(defaultQuery), true
	case key.Matches(msg, k.Filter):
		// The list gets no f, which pages down there.
		return ui.OpenFilter(filterform.FiltersTab), true
	case key.Matches(msg, k.Select):
		return s.open(), true
	case key.Matches(msg, k.Open):
		if n, ok := s.feed.Selected(); ok {
			return ui.Open(n.Subject.WebURL), true
		}
		return nil, true
	case key.Matches(msg, k.MarkRead, k.MarkDone, k.MarkAllRead):
		if cmd, refused := s.gate().Refuse(ui.ActMarkRead, nil); refused {
			return cmd, true
		}
		switch {
		case key.Matches(msg, k.MarkRead):
			return s.ask(s.markRead), true
		case key.Matches(msg, k.MarkDone):
			return s.ask(s.markDone), true
		}
		return s.ask(s.markAllRead), true
	}
	return nil, false
}

// gate decides what the token may do with the notifications.
func (s *Section) gate() ui.Gate {
	return ui.Gate{Token: s.voice.Token, Icons: s.icons}
}

// mark is a change to threads of the inbox, asked as a question: name
// names them in a note, and ids tells which they are.
type mark struct {
	ask       ui.Confirm
	name, ids string
}

// ask asks the mark that now returns, if there is one, in a modal of its
// own. By the time the user says yes, the list may have changed behind
// the question, so it asks now again, and makes the change only if it is
// still the one asked of the same threads.
func (s *Section) ask(now func() (mark, bool)) tea.Cmd {
	m, ok := now()
	if !ok {
		return nil
	}
	c := ui.Recheck(m.ask.Question, m.name, func() (ui.Confirm, bool, tea.Cmd) {
		again, ok := now()
		return again.ask, ok && again.ids == m.ids, nil
	})
	return ui.OpenModal(ui.NewConfirmModal(c, s.icons))
}

// markRead marks the unread thread under the cursor read.
func (s *Section) markRead() (mark, bool) {
	n, ok := s.feed.Selected()
	if !ok || !n.Unread {
		return mark{}, false
	}
	return mark{
		ask: ui.Confirm{
			Question: "Mark " + threadName(n) + " as read?",
			Run:      func() tea.Cmd { return s.do(s.svc.MarkRead(n.ID), "mark read") },
		},
		name: threadName(n), ids: n.ID,
	}, true
}

// markDone marks the thread under the cursor done, which drops it from
// the inbox.
func (s *Section) markDone() (mark, bool) {
	n, ok := s.feed.Selected()
	if !ok {
		return mark{}, false
	}
	return mark{
		ask: ui.Confirm{
			Question: "Mark " + threadName(n) + " as done?",
			Run:      func() tea.Cmd { return s.do(s.svc.MarkDone(n.ID), "mark done") },
		},
		name: threadName(n), ids: n.ID,
	}, true
}

// markAllRead marks every thread read, loaded or not, that was updated
// until the newest in the list, so those the user hasn't seen stay unread.
// The threads are those unread in the list and the newest, so one that
// arrives or is read meanwhile changes what the question is about.
func (s *Section) markAllRead() (mark, bool) {
	if s.feed.Len() == 0 {
		return mark{}, false
	}
	var newest time.Time
	var ids strings.Builder
	for i := range s.feed.Len() {
		n, ok := s.feed.Item(i)
		if !ok {
			continue
		}
		if n.UpdatedAt.After(newest) {
			newest = n.UpdatedAt
		}
		if n.Unread {
			ids.WriteString(n.ID)
			ids.WriteByte(' ')
		}
	}
	ids.WriteString(newest.Format(time.RFC3339Nano))
	return mark{
		ask: ui.Confirm{
			Question: "Mark all notifications as read?",
			Run:      func() tea.Cmd { return s.do(s.svc.MarkAllRead(newest), "mark all read") },
		},
		name: "The inbox", ids: ids.String(),
	}, true
}

// threadName names what thread n is about, such as "eggzec/gh-tui#12", or
// by its title in its repository when it has no number, such as a release.
func threadName(n core.Notification) string {
	if n.Subject.Number > 0 {
		return n.Repo.String() + "#" + strconv.Itoa(n.Subject.Number)
	}
	return "\"" + ui.OneLine(n.Subject.Title) + "\" in " + n.Repo.String()
}

// open opens what the selected thread is about in the app, and marks it
// read if opening marks threads read.
func (s *Section) open() tea.Cmd {
	n, ok := s.feed.Selected()
	if !ok {
		return nil
	}
	cmd := s.opener.Open(n)
	if s.opener.MarksRead() && n.Unread {
		cmd = tea.Batch(cmd, s.do(s.svc.MarkRead(n.ID), "mark read"))
	}
	return cmd
}

// readAhead reads ahead what the threads around the cursor are about, once
// the list has started, while it is on view.
func (s *Section) readAhead() tea.Cmd {
	if !s.started || !s.feed.Focused() {
		return nil
	}
	if s.feed.Len() == 0 && !s.feed.Settled() {
		// The list hasn't loaded.
		return s.opener.ReadAhead(nil, 0)
	}
	return s.opener.ReadAhead(s.feed.Item, s.feed.Index())
}

// do shows the change op already made to the cache and sends it.
func (s *Section) do(op *optimistic.Op, what string) tea.Cmd {
	return tea.Batch(s.reload(), ui.Do(s.ctx, ui.NotificationsTitle, op, what))
}

// reload reads the loaded pages again. Before Init there is nothing to
// reload, and the first fetch happens then.
func (s *Section) reload() tea.Cmd {
	if !s.started {
		return nil
	}
	return s.feed.Reload()
}

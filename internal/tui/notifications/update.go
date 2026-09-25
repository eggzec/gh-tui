package notifications

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update handles the section's keys, sync events and finished changes, and
// passes everything else to the list.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(ui.AheadMsg); ok {
		return s.opener.Rested(msg)
	}
	cmd := s.update(msg)
	if off := s.offline.Notify(); off != nil {
		cmd = tea.Batch(cmd, off)
	}
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
	case ui.DoneMsg:
		// A failed change was rolled back in the cache; a successful one
		// was applied again.
		if msg.From != ui.NotificationsTitle {
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
	case key.Matches(msg, k.ClearFilter) && s.filtered():
		return s.setFilter(defaultQuery), true
	case key.Matches(msg, k.Select):
		return s.open(), true
	case key.Matches(msg, k.Open):
		if n, ok := s.feed.Selected(); ok {
			return ui.Open(n.Subject.WebURL), true
		}
		return nil, true
	case key.Matches(msg, k.MarkRead):
		n, ok := s.feed.Selected()
		if !ok || !n.Unread {
			return nil, true
		}
		return s.do(s.svc.MarkRead(n.ID), "mark read"), true
	case key.Matches(msg, k.MarkDone):
		n, ok := s.feed.Selected()
		if !ok {
			return nil, true
		}
		return s.do(s.svc.MarkDone(n.ID), "mark done"), true
	case key.Matches(msg, k.MarkAllRead):
		if s.feed.Len() == 0 {
			return nil, true
		}
		return s.do(s.svc.MarkAllRead(), "mark all read"), true
	}
	return nil, false
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

// readAhead reads ahead what the first threads and the one under the
// cursor are about, once the list has started.
func (s *Section) readAhead() tea.Cmd {
	if !s.started {
		return nil
	}
	n, ok := s.feed.Selected()
	return s.opener.ReadAhead(s.feed.Item, n, ok)
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

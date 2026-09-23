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
		return s.reload(), true
	case key.Matches(msg, k.Filter):
		s.all.Store(!s.all.Load())
		s.renderHeader()
		return s.feed.Reset(), true
	case key.Matches(msg, k.Select):
		return s.open(true), true
	case key.Matches(msg, k.Open):
		return s.open(false), true
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

// open opens the selected thread in the browser, and marks it read if read
// is set.
func (s *Section) open(read bool) tea.Cmd {
	n, ok := s.feed.Selected()
	if !ok {
		return nil
	}
	cmd := ui.Open(n.Subject.WebURL)
	if read && n.Unread {
		cmd = tea.Batch(cmd, s.do(s.svc.MarkRead(n.ID), "mark read"))
	}
	return cmd
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

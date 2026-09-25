package notifications

import (
	"regexp"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update handles the section's keys, sync events and finished changes, and
// passes everything else to the list.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	cmd := s.update(msg)
	if off := s.offline.Notify(); off != nil {
		return tea.Batch(cmd, off)
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
		return s.reload(), true
	case key.Matches(msg, k.ClearFilter) && s.filtered():
		return s.setFilter(defaultQuery), true
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
// is set. With read set, a thread about a workflow run opens the Actions
// modal on the runs of its branch that ended the same way instead.
func (s *Section) open(read bool) tea.Cmd {
	n, ok := s.feed.Selected()
	if !ok {
		return nil
	}
	cmd := ui.Open(n.Subject.WebURL)
	if f, ok := runFilter(n.Subject); ok && read {
		msg := ui.OpenActionsMsg{Repo: n.Repo, Filter: f}
		cmd = func() tea.Msg { return msg }
	}
	if read && n.Unread {
		cmd = tea.Batch(cmd, s.do(s.svc.MarkRead(n.ID), "mark read"))
	}
	return cmd
}

// runTitle is the title GitHub gives a notification of a workflow run,
// such as "CI workflow run failed for main branch".
var runTitle = regexp.MustCompile(`^(.+) workflow run (\w+) for (.+) branch$`)

// runFilter selects the runs a notification of a check suite is about: its
// subject has no URL, but its title names the branch and how the run
// ended, which the runs list filters by without another request.
func runFilter(sub core.Subject) (core.RunFilter, bool) {
	if sub.Type != core.SubjectCheckSuite {
		return core.RunFilter{}, false
	}
	m := runTitle.FindStringSubmatch(sub.Title)
	if m == nil {
		return core.RunFilter{}, false
	}
	f := core.RunFilter{Branch: m[3]}
	switch m[2] {
	case "failed":
		f.Status = string(core.ConclusionFailure)
	case "succeeded":
		f.Status = string(core.ConclusionSuccess)
	case "cancelled":
		f.Status = string(core.ConclusionCancelled)
	}
	return f, true
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

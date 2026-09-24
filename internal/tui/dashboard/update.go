package dashboard

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update handles the dashboard's keys, its reads and the changes to the
// inbox, and passes everything else to the repositories, whose lists and
// filter ignore the messages of others.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	cmd, all := s.update(msg)
	if all {
		s.render()
	} else {
		s.renderPane(reposPane)
		s.compose()
	}
	if off := s.offline.Notify(); off != nil {
		return tea.Batch(cmd, off)
	}
	return cmd
}

// update handles msg, and reports whether more than the repositories may
// look different.
func (s *Section) update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case loadedMsg:
		return s.loaded(msg), true
	case tea.KeyPressMsg:
		return s.press(msg), true
	case ui.SyncMsg:
		// The poll refreshed the cache of the inbox.
		if msg.Key != notifications.SyncKey || msg.Err != nil {
			return nil, false
		}
		s.readInboxCache()
		return nil, true
	case ui.DoneMsg:
		// Marking threads read changes the unread ones.
		if msg.From != ui.NotificationsTitle {
			return nil, false
		}
		s.readInboxCache()
		return nil, true
	}
	// The calendar has no messages of its own.
	return s.repos.update(msg), false
}

// press handles a key: the filter takes every key while it is open, then
// the focused pane takes its own, then the dashboard's, then the pane's
// navigation.
func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
	if s.repos.filtering {
		return s.repos.update(msg)
	}
	if !s.focused {
		return nil
	}
	if cmd, ok := s.pressPane(msg); ok {
		return cmd
	}
	k := &s.keys
	switch {
	case key.Matches(msg, k.Next):
		s.focusPane((s.focus + 1) % numPanes)
		return nil
	case key.Matches(msg, k.Prev):
		s.focusPane((s.focus + numPanes - 1) % numPanes)
		return nil
	case key.Matches(msg, k.Refresh):
		return s.refresh()
	case key.Matches(msg, k.Here):
		if s.here == (core.RepoRef{}) {
			return nil
		}
		return selectRepo(s.here)
	}
	if p := k.pane(msg); p >= 0 {
		s.focusPane(p)
		return nil
	}
	switch s.focus {
	case reposPane:
		return s.repos.update(msg)
	case calendarPane:
		var cmd tea.Cmd
		s.cal, cmd = s.cal.Update(msg)
		return cmd
	default:
		return nil
	}
}

// pressPane handles the keys of the focused pane, and reports whether msg
// was one.
func (s *Section) pressPane(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := &s.keys
	switch s.focus {
	case pinnedPane:
		c := &s.pinned
		switch {
		case key.Matches(msg, k.Left):
			c.move(-1)
		case key.Matches(msg, k.Right):
			c.move(1)
		case key.Matches(msg, k.Up):
			c.move(-c.cols)
		case key.Matches(msg, k.Down):
			c.move(c.cols)
		case key.Matches(msg, k.Select):
			if it, ok := c.selected(); ok {
				return selectRepo(it.repo.Ref), true
			}
		case key.Matches(msg, k.Open):
			if it, ok := c.selected(); ok {
				return ui.Open(repoURL(it.repo)), true
			}
		default:
			return nil, false
		}
		return nil, true
	case reposPane:
		t := &s.repos
		switch {
		case key.Matches(msg, k.NextOwner):
			return t.switchTab(1), true
		case key.Matches(msg, k.PrevOwner):
			return t.switchTab(-1), true
		case key.Matches(msg, k.Filter):
			return t.openFilter(), true
		case key.Matches(msg, k.Select):
			if r, ok := t.selected(); ok {
				return selectRepo(r.Ref), true
			}
			return nil, true
		case key.Matches(msg, k.Open):
			if r, ok := t.selected(); ok {
				return ui.Open(repoURL(r)), true
			}
			return nil, true
		}
	case workPane:
		w := &s.tasks
		switch {
		case key.Matches(msg, k.Up):
			w.move(-1)
		case key.Matches(msg, k.Down):
			w.move(1)
		case key.Matches(msg, k.Select):
			if hit, ok := w.selected(); ok {
				return openHit(hit), true
			}
		case key.Matches(msg, k.Open):
			if hit, ok := w.selected(); ok {
				return ui.Open(hit.Issue.URL), true
			}
		default:
			return nil, false
		}
		return nil, true
	case inboxPane:
		if key.Matches(msg, k.Select) {
			return func() tea.Msg { return ui.ShowMsg{Title: ui.NotificationsTitle} }, true
		}
	default:
	}
	return nil, false
}

// focusPane moves the focus to pane p.
func (s *Section) focusPane(p paneID) {
	s.repos.blur()
	s.cal.Blur()
	s.focus = p
	if !s.focused {
		return
	}
	switch p {
	case reposPane:
		s.repos.focus()
	case calendarPane:
		s.cal.Focus()
	default:
	}
}

// openHit returns the command that opens the pull request or issue of hit
// in its modal.
func openHit(hit core.SearchHit) tea.Cmd {
	is := hit.Issue
	var msg tea.Msg = ui.OpenIssueMsg{Repo: is.Repo, Number: is.Number}
	if hit.Kind == core.SearchPulls {
		msg = ui.OpenPullMsg{Repo: is.Repo, Number: is.Number}
	}
	return func() tea.Msg { return msg }
}

// repoURL is the page of r on GitHub.
func repoURL(r core.Repo) string {
	if r.URL != "" {
		return r.URL
	}
	return "https://github.com/" + r.Ref.String()
}

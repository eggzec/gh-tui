package dashboard

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update handles the dashboard's keys, its reads and the changes to the
// inbox, and passes everything else to the repositories, whose lists
// ignore the messages of others.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(ui.AheadMsg); ok {
		// The opener may be the notifications screen's too, which reads
		// ahead while it is on view instead.
		if !s.focused {
			return nil
		}
		return tea.Batch(s.opener.Rested(msg), s.ahead.Rested(msg), s.aheadRepos.Rested(msg), s.aheadPinned.Rested(msg))
	}
	updating := s.updating()
	cmd, all := s.update(msg)
	switch {
	case all:
		s.render()
	default:
		if s.updating() != updating {
			// The profile says whether the repositories are read again.
			s.head = s.profile()
		}
		s.renderPane(reposPane)
		s.compose()
	}
	if ahead := s.readAhead(); ahead != nil {
		cmd = tea.Batch(cmd, ahead)
	}
	if ahead := s.readWorkAhead(); ahead != nil {
		cmd = tea.Batch(cmd, ahead)
	}
	if ahead := tea.Batch(s.readReposAhead(), s.readPinnedAhead()); ahead != nil {
		cmd = tea.Batch(cmd, ahead)
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
	case ui.SettingsMsg:
		s.configure(msg.Config)
		return nil, false
	case ui.SyncMsg:
		// The poll refreshed the cache of the inbox.
		if msg.Key != notifications.SyncKey || msg.Err != nil {
			return nil, false
		}
		s.readInboxCache()
		return nil, true
	case ui.OnlineMsg:
		return s.online(msg), true
	case ui.ImagesMsg:
		// The profile may have gained or lost the avatar's rows.
		s.layout()
		return nil, true
	case ui.AccessMsg:
		// The inbox the token was refused, or failed to read, it may
		// read now.
		if s.inbox == nil || !s.started || s.notes.loading || s.notes.ok && s.notes.err == nil {
			return nil, true
		}
		return s.readInbox(true), true
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

// press handles a key: the focused pane takes its own, then the
// dashboard's, then the pane's navigation.
func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
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
	case s.wide && key.Matches(msg, k.Zoom):
		s.setZoom(!s.zoom)
		return nil
	case s.zoomed() && key.Matches(msg, k.Back):
		s.setZoom(false)
		return nil
	case key.Matches(msg, k.Refresh):
		return s.refresh()
	case key.Matches(msg, k.Here):
		if s.here == (core.RepoRef{}) {
			return nil
		}
		s.aheadPinned.Opened(s.here)
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
				s.aheadPinned.Opened(it.repo.Ref)
				return selectRepo(it.repo.Ref), true
			}
		case key.Matches(msg, k.Open):
			if it, ok := c.selected(); ok {
				return ui.Open(s.repoURL(it.repo)), true
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
		case key.Matches(msg, k.ClearFilter):
			if !t.filter().active() {
				return nil, true
			}
			return t.setFilter(""), true
		case key.Matches(msg, k.Filter, k.Sort):
			// The app opens the filter. Its keys don't reach the list,
			// whose page down f is too.
			return nil, true
		case key.Matches(msg, k.Select):
			if r, ok := t.selected(); ok {
				s.aheadRepos.Opened(r.Ref)
				return selectRepo(r.Ref), true
			}
			return nil, true
		case key.Matches(msg, k.Open):
			if r, ok := t.selected(); ok {
				return ui.Open(s.repoURL(r)), true
			}
			return nil, true
		}
	case workPane:
		w := &s.tasks
		switch {
		case key.Matches(msg, k.NextOwner):
			w.switchTab(1)
			s.readTabNow()
		case key.Matches(msg, k.PrevOwner):
			w.switchTab(-1)
			s.readTabNow()
		case key.Matches(msg, k.Up):
			w.move(-1)
		case key.Matches(msg, k.Down):
			w.move(1)
		case key.Matches(msg, k.Select):
			if hit, ok := w.selected(); ok {
				return s.openHit(hit, false), true
			}
		case key.Matches(msg, k.Checks):
			if hit, ok := w.selected(); ok && hit.Kind == core.SearchPulls {
				return s.openHit(hit, true), true
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
		l := &s.threads
		switch {
		case key.Matches(msg, k.Up):
			l.move(-1)
		case key.Matches(msg, k.Down):
			l.move(1)
		case key.Matches(msg, k.Select):
			return s.openThread(), true
		case key.Matches(msg, k.Open):
			if n, ok := l.selected(); ok {
				return ui.Open(n.Subject.WebURL), true
			}
		default:
			return nil, false
		}
		return nil, true
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

// repoURL is the page of r on GitHub.
func (s *Section) repoURL(r core.Repo) string {
	if r.URL != "" {
		return r.URL
	}
	return ui.WebURL(s.host, r.Ref.String())
}

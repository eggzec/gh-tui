package issues

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update implements ui.Section.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(ui.AheadMsg); ok {
		return s.ahead.Rested(msg)
	}
	cmd := s.update(msg)
	if off := s.offline.Notify(); off != nil {
		cmd = tea.Batch(cmd, off)
	}
	if ahead := s.readAhead(); ahead != nil {
		cmd = tea.Batch(cmd, ahead)
	}
	if others := s.readOthers(); others != nil {
		cmd = tea.Batch(cmd, others)
	}
	if ms := s.readMilestones(); ms != nil {
		cmd = tea.Batch(cmd, ms)
	}
	return cmd
}

func (s *Section) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.RepoMsg:
		return s.setRepo(msg.Repo)
	case tea.KeyPressMsg:
		return s.press(msg)
	case ui.OpenIssueMsg:
		return s.openDetail(msg.Repo, msg.Number, nil)
	case changedMsg:
		if !s.hasRepo || msg.repo != s.repo {
			return nil
		}
		return s.reload()
	case ui.SyncMsg:
		return s.sync(msg)
	case ui.CapsMsg:
		return s.setCaps(msg)
	case viewerMsg:
		s.viewer = msg.login
		return nil
	case ui.DoneMsg:
		return tea.Batch(s.done(msg), s.forward(msg))
	}
	return s.forward(msg)
}

// forward passes msg to the list, which ignores what isn't its own. A
// repository without issues has no list to pass it to.
func (s *Section) forward(msg tea.Msg) tea.Cmd {
	if s.issuesOff() {
		return nil
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

// sync reloads the list when the issues of the repository changed. Poll
// invalidated them, so the reads revalidate with GitHub. A failed poll
// changed nothing.
func (s *Section) sync(msg ui.SyncMsg) tea.Cmd {
	if msg.Err != nil || !s.live() || msg.Key != issuesvc.SyncKey(s.repo) {
		return nil
	}
	return s.list.Reload()
}

// setRepo shows the issues of repo. It arrives before Init too, so the
// section only loads them once started.
func (s *Section) setRepo(repo core.RepoRef) tea.Cmd {
	if s.hasRepo && repo == s.repo {
		return nil
	}
	s.repo, s.hasRepo = repo, true
	s.caps = ui.CachedCaps(s.repos, repo)
	// The rate limit may be another's.
	s.ahead.Resume()
	s.others.Reset(s.ctx)
	s.milestonesRead = false
	// Other repositories have other labels.
	s.chips = newChipCache(s.rows)
	return s.resetList()
}

// setCaps takes what the viewer may do in the repository. If it turned its
// issues off, the list stops reading them, and it starts if they are on
// after all.
func (s *Section) setCaps(msg ui.CapsMsg) tea.Cmd {
	if !s.hasRepo || msg.Repo != s.repo {
		return nil
	}
	wasOff := s.issuesOff()
	s.caps = msg.Caps
	switch off := s.issuesOff(); {
	case off && !wasOff:
		// The reads ahead of the other tabs stop too.
		s.cancelList()
		s.others.Reset(s.ctx)
		s.renderChrome()
	case wasOff && !off:
		return s.resetList()
	}
	return nil
}

// viewerMsg carries the login of the signed-in user.
type viewerMsg struct {
	login string
}

// loadViewer reads who the viewer is, unless the section knows.
func (s *Section) loadViewer() tea.Cmd {
	if s.readViewer == nil || s.viewer != "" {
		return nil
	}
	read, ctx := s.readViewer, s.ctx
	return func() tea.Msg {
		login, err := read(ctx)
		if err != nil {
			// The viewer's own issues are then taken for anyone's.
			return nil
		}
		return viewerMsg{login: login}
	}
}

func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
	if !s.hasRepo || s.issuesOff() {
		return nil
	}
	k := s.keys
	switch {
	case key.Matches(msg, k.Select):
		it, ok := s.list.Selected()
		if !ok {
			return nil
		}
		return s.openDetail(s.repo, it.Number, &it)
	case key.Matches(msg, k.Close):
		return s.setState(core.StateClosed)
	case key.Matches(msg, k.Reopen):
		return s.setState(core.StateOpen)
	case key.Matches(msg, k.NextTab):
		return s.show(nextTab(s.tab, 1), s.query)
	case key.Matches(msg, k.PrevTab):
		return s.show(nextTab(s.tab, -1), s.query)
	case key.Matches(msg, k.ClearFilter):
		return s.show(s.tab, "")
	case key.Matches(msg, k.Refresh):
		s.svc.Invalidate(s.repo)
		return s.list.Reload()
	case key.Matches(msg, k.Open):
		if it, ok := s.list.Selected(); ok && it.URL != "" {
			return ui.Open(it.URL)
		}
		return nil
	}
	return s.forward(msg)
}

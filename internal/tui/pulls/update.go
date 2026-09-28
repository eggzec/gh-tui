package pulls

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
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
	return cmd
}

func (s *Section) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.RepoMsg:
		return s.setRepo(msg.Repo)
	case ui.SyncMsg:
		return s.sync(msg)
	case ui.CapsMsg:
		if s.hasRepo && msg.Repo == s.repo {
			s.caps = msg.Caps
		}
		return nil
	case ui.OpenPullMsg:
		return s.openDetail(msg.Repo, msg.Number, nil, msg.Checks, msg.ShowRepo, msg.Pause)
	case changedMsg:
		if !s.hasRepo || msg.repo != s.repo {
			return nil
		}
		return s.reload()
	case ui.DoneMsg:
		// The change was confirmed or rolled back; either way the cache
		// has the outcome.
		if msg.From != ui.PullsTitle {
			return nil
		}
		return s.reload()
	case tea.KeyPressMsg:
		if cmd, ok := s.mutate(msg); ok {
			return cmd
		}
		return s.press(msg)
	}
	return s.forward(msg)
}

// setRepo shows the pull requests of repo, once the section has started.
func (s *Section) setRepo(repo core.RepoRef) tea.Cmd {
	if s.hasRepo && repo == s.repo {
		return nil
	}
	s.repo, s.hasRepo = repo, true
	s.caps = ui.CachedCaps(s.repos, repo)
	// The rate limit may be another's.
	s.ahead.Resume()
	s.others.Reset(s.ctx, repo.String())
	s.renderHeader()
	if !s.started {
		return nil
	}
	return s.newFeed()
}

// sync reloads what is shown when the pull requests of the repository
// changed. Poll invalidated them, so the reads reach GitHub. A failed poll
// changed nothing.
func (s *Section) sync(msg ui.SyncMsg) tea.Cmd {
	if msg.Err != nil || !s.started || !s.hasRepo || msg.Key != pulls.SyncKey(s.repo) {
		return nil
	}
	return s.reload()
}

func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
	if s.feed == nil {
		return nil
	}
	k := s.keys
	switch {
	case key.Matches(msg, k.Select):
		if pr, ok := s.feed.Selected(); ok {
			return s.openDetail(s.repo, pr.Number, &pr, false, false, nil)
		}
		return nil
	case key.Matches(msg, k.Checks):
		if pr, ok := s.feed.Selected(); ok {
			return s.openDetail(s.repo, pr.Number, &pr, true, false, nil)
		}
		return nil
	case key.Matches(msg, k.NextTab):
		s.others.Arm()
		return s.show(nextTab(s.tab, 1), s.query)
	case key.Matches(msg, k.PrevTab):
		s.others.Arm()
		return s.show(nextTab(s.tab, -1), s.query)
	case key.Matches(msg, k.ClearFilter):
		return s.show(s.tab, "")
	case key.Matches(msg, k.Refresh):
		s.svc.Invalidate(s.repo)
		return s.feed.Reload()
	case key.Matches(msg, k.Open):
		if pr, ok := s.feed.Selected(); ok && pr.URL != "" {
			return ui.Open(pr.URL)
		}
		return nil
	}
	var cmd tea.Cmd
	*s.feed, cmd = s.feed.Update(msg)
	return cmd
}

// forward passes msg to the feed, which ignore what isn't theirs.
func (s *Section) forward(msg tea.Msg) tea.Cmd {
	if s.feed == nil {
		return nil
	}
	var cmd tea.Cmd
	*s.feed, cmd = s.feed.Update(msg)
	return cmd
}

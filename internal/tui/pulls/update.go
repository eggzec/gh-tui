package pulls

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update implements ui.Section.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.RepoMsg:
		return s.setRepo(msg.Repo)
	case tea.KeyPressMsg:
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
	s.renderHeader()
	if !s.started {
		return nil
	}
	return s.newFeed()
}

func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
	if s.feed == nil {
		return nil
	}
	k := s.keys
	switch {
	case key.Matches(msg, k.Filter):
		s.filter = nextFilter(s.filter)
		return s.newFeed()
	case key.Matches(msg, k.Refresh):
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

// forward passes msg to the bubbles, which ignore what isn't theirs.
func (s *Section) forward(msg tea.Msg) tea.Cmd {
	if s.feed == nil {
		return nil
	}
	var cmd tea.Cmd
	*s.feed, cmd = s.feed.Update(msg)
	return cmd
}

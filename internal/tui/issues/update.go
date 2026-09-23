package issues

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
	case issueMsg:
		return s.gotIssue(msg)
	case ui.DoneMsg:
		return tea.Batch(s.done(msg), s.forward(msg))
	}
	return s.forward(msg)
}

// forward passes msg to the bubbles, which ignore what isn't theirs.
func (s *Section) forward(msg tea.Msg) tea.Cmd {
	var cmd, detail tea.Cmd
	s.list, cmd = s.list.Update(msg)
	if s.inDetail {
		s.detail, detail = s.detail.Update(msg)
	}
	return tea.Batch(cmd, detail)
}

// setRepo shows the issues of repo. It arrives before Init too, so the
// section only loads them once started.
func (s *Section) setRepo(repo core.RepoRef) tea.Cmd {
	if s.hasRepo && repo == s.repo {
		return nil
	}
	s.back()
	s.repo, s.hasRepo = repo, true
	// Other repositories have other labels.
	clear(s.chips)
	return s.resetList()
}

func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
	if !s.hasRepo {
		return nil
	}
	if s.inDetail {
		return s.pressDetail(msg)
	}
	k := s.keys
	switch {
	case key.Matches(msg, k.Select):
		return s.open()
	case key.Matches(msg, k.Close):
		return s.setState(core.StateClosed)
	case key.Matches(msg, k.Reopen):
		return s.setState(core.StateOpen)
	case key.Matches(msg, k.Filter):
		s.filter = nextFilter(s.filter)
		return s.resetList()
	case key.Matches(msg, k.Refresh):
		return s.list.Reload()
	case key.Matches(msg, k.Open):
		if it, ok := s.list.Selected(); ok && it.URL != "" {
			return ui.Open(it.URL)
		}
		return nil
	}
	return s.forward(msg)
}

func (s *Section) pressDetail(msg tea.KeyPressMsg) tea.Cmd {
	k := s.keys
	switch {
	case key.Matches(msg, k.Back):
		s.back()
		return nil
	case key.Matches(msg, k.Close):
		return s.setState(core.StateClosed)
	case key.Matches(msg, k.Reopen):
		return s.setState(core.StateOpen)
	case key.Matches(msg, k.Refresh):
		return tea.Batch(s.detail.Reload(), s.get())
	case key.Matches(msg, k.Open):
		if s.issue.URL != "" {
			return ui.Open(s.issue.URL)
		}
		return nil
	}
	var cmd tea.Cmd
	s.detail, cmd = s.detail.Update(msg)
	return cmd
}

package issues

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
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
	case ui.SyncMsg:
		return s.sync(msg)
	case ui.DoneMsg:
		return tea.Batch(s.done(msg), s.forward(msg))
	case prompt.SubmitMsg, prompt.CancelMsg:
		return s.promptDone(msg)
	case tea.PasteMsg:
		if s.composing != composeNone {
			var cmd tea.Cmd
			s.prompt, cmd = s.prompt.Update(msg)
			return cmd
		}
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

// sync reloads what is shown when the issues of the repository changed.
// Poll invalidated them, so the reads revalidate with GitHub. A failed poll
// changed nothing.
func (s *Section) sync(msg ui.SyncMsg) tea.Cmd {
	if msg.Err != nil || !s.started || !s.hasRepo || msg.Key != issuesvc.SyncKey(s.repo) {
		return nil
	}
	cmd := s.list.Reload()
	if !s.inDetail {
		return cmd
	}
	return tea.Batch(cmd, s.detail.Reload(), s.get())
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

func (s *Section) pressDetail(msg tea.KeyPressMsg) tea.Cmd {
	if s.composing != composeNone {
		// Every key is typing, even the section's own.
		var cmd tea.Cmd
		s.prompt, cmd = s.prompt.Update(msg)
		return cmd
	}
	k := s.keys
	switch {
	case key.Matches(msg, k.Comment):
		return s.compose(composeComment)
	case key.Matches(msg, k.Label):
		return s.compose(composeLabels)
	case key.Matches(msg, k.Back):
		s.back()
		return nil
	case key.Matches(msg, k.Close):
		return s.setState(core.StateClosed)
	case key.Matches(msg, k.Reopen):
		return s.setState(core.StateOpen)
	case key.Matches(msg, k.Refresh):
		s.svc.Invalidate(s.repo)
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

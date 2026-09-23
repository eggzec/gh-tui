package issues

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// target returns the issue that actions apply to: the open one in the
// detail, or the selected one in the list.
func (s *Section) target() (core.Issue, bool) {
	if s.inDetail {
		return s.issue, true
	}
	return s.list.Selected()
}

// setState closes or reopens the target issue. The service shows the change
// in its cache at once, so the section reloads from it, then sends the
// change. Only an open issue closes and only a closed one reopens.
func (s *Section) setState(state core.State) tea.Cmd {
	from := core.StateOpen
	if state == core.StateOpen {
		from = core.StateClosed
	}
	it, ok := s.target()
	if !ok || it.State != from {
		return nil
	}
	var op *optimistic.Op
	verb := "close"
	if state == core.StateClosed {
		op = s.svc.Close(s.repo, it.Number)
	} else {
		verb = "reopen"
		op = s.svc.Reopen(s.repo, it.Number)
	}
	what := verb + " #" + strconv.Itoa(it.Number)
	return tea.Batch(s.reload(), ui.Do(s.ctx, ui.IssuesTitle, op, what))
}

// done reloads after a change this section sent, to show GitHub's answer or
// the rollback. The thread reloads too, since the change may be a comment.
func (s *Section) done(msg ui.DoneMsg) tea.Cmd {
	if msg.From != ui.IssuesTitle {
		return nil
	}
	if s.inDetail {
		return tea.Batch(s.reload(), s.detail.Reload())
	}
	return s.reload()
}

// reload shows the list, and the open issue's header, from the cache again.
func (s *Section) reload() tea.Cmd {
	cmd := s.list.Reload()
	if !s.inDetail {
		return cmd
	}
	it, ok := s.svc.CachedGet(s.repo, s.issue.Number)
	if !ok {
		return cmd
	}
	s.issue = it
	return tea.Batch(cmd, s.detail.SetDocument(s.header(it), it.Body))
}

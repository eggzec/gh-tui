package issues

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// target returns the issue that the list keys apply to: the selected one.
func (s *Section) target() (core.Issue, bool) {
	return s.list.Selected()
}

// stateChange starts closing or reopening issue it of repo: the service
// shows the change in its cache at once and returns the op that sends it,
// named by what. Only an open issue closes and only a closed one reopens,
// and ok is false for the others.
func stateChange(svc Service, repo core.RepoRef, it core.Issue, state core.State) (op *optimistic.Op, what string, ok bool) {
	from, verb := core.StateOpen, "close"
	if state == core.StateOpen {
		from, verb = core.StateClosed, "reopen"
	}
	if it.State != from {
		return nil, "", false
	}
	if state == core.StateClosed {
		op = svc.Close(repo, it.Number)
	} else {
		op = svc.Reopen(repo, it.Number)
	}
	return op, verb + " #" + strconv.Itoa(it.Number), true
}

// setState closes or reopens the selected issue. The list shows the change
// from the cache at once, then it is sent.
func (s *Section) setState(state core.State) tea.Cmd {
	it, ok := s.target()
	if !ok {
		return nil
	}
	op, what, ok := stateChange(s.svc, s.repo, it, state)
	if !ok {
		return nil
	}
	return tea.Batch(s.reload(), ui.Do(s.ctx, ui.IssuesTitle, op, what))
}

// done reloads the list after a change this section or its modal sent, to
// show GitHub's answer or the rollback.
func (s *Section) done(msg ui.DoneMsg) tea.Cmd {
	if msg.From != ui.IssuesTitle {
		return nil
	}
	return s.reload()
}

// reload shows the list from the cache again.
func (s *Section) reload() tea.Cmd {
	return s.list.Reload()
}

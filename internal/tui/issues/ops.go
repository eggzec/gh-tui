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

// stateChange starts closing or reopening issue it: the service shows the
// change in its cache at once and returns the op that sends it, named by
// what. Only an open issue closes and only a closed one reopens, and op is
// nil for the others; it is nil too when g refuses the change, and
// refusal says why.
func stateChange(svc Service, g ui.Gate, it core.Issue, state core.State) (op *optimistic.Op, what string, refusal tea.Cmd) {
	from, verb, a := core.StateOpen, "close", ui.ActClose
	if state == core.StateOpen {
		from, verb, a = core.StateClosed, "reopen", ui.ActReopen
	}
	if it.State != from {
		return nil, "", nil
	}
	if cmd, refused := g.Refuse(a, &it); refused {
		return nil, "", cmd
	}
	if state == core.StateClosed {
		op = svc.Close(g.Repo, it.Number)
	} else {
		op = svc.Reopen(g.Repo, it.Number)
	}
	return op, verb + " #" + strconv.Itoa(it.Number), nil
}

// setState closes or reopens the selected issue. The list shows the change
// from the cache at once, then it is sent.
func (s *Section) setState(state core.State) tea.Cmd {
	it, ok := s.target()
	if !ok {
		return nil
	}
	op, what, refusal := stateChange(s.svc, s.gate(), it, state)
	if op == nil {
		return refusal
	}
	return tea.Batch(s.reload(), ui.Do(s.ctx, ui.IssuesTitle, op, what))
}

// gate decides what the viewer may do in the repository of the list.
func (s *Section) gate() ui.Gate {
	return ui.Gate{Repo: s.repo, Caps: s.caps, Viewer: s.viewer}
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

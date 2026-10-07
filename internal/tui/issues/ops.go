package issues

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// target returns the issue that the list keys apply to: the selected one.
func (s *Section) target() (core.Issue, bool) {
	return s.list.Selected()
}

// stateVerb returns the verb that moves it to state, if it applies: only
// an open issue closes and only a closed one reopens. ok is unset for the
// others, and when g refuses the change, which refusal then says why.
func stateVerb(g ui.Gate, it core.Issue, state core.State) (verb string, ok bool, refusal tea.Cmd) {
	from, verb, a := core.StateOpen, "Close", ui.ActClose
	if state == core.StateOpen {
		from, verb, a = core.StateClosed, "Reopen", ui.ActReopen
	}
	if it.State != from {
		return "", false, nil
	}
	if cmd, refused := g.Refuse(a, &it); refused {
		return "", false, cmd
	}
	return verb, true, nil
}

// stateChange returns closing or reopening issue it as a question. Its
// run, once the user says yes, shows the change in the cache at once and
// returns send, the command that sends it. By then the issue may have
// changed and so may what the viewer may do, so run checks again the
// issue and the gate that now returns, and sends nothing when the change
// no longer applies, such as for another issue or the same number of
// another repository. ok is unset when the change doesn't apply now, and
// refusal says why when g refuses it.
func stateChange(svc Service, g ui.Gate, it core.Issue, state core.State,
	now func() (core.Issue, ui.Gate, bool), send func(op *optimistic.Op, what string) tea.Cmd,
) (c ui.Confirm, ok bool, refusal tea.Cmd) {
	verb, ok, refusal := stateVerb(g, it, state)
	if !ok {
		return ui.Confirm{}, false, refusal
	}
	repo, number := g.Repo, it.Number
	n := "#" + strconv.Itoa(number)
	return ui.Confirm{
		Question: verb + " issue " + n + "?",
		Run: func() tea.Cmd {
			it, g, found := now()
			var applies bool
			var refused tea.Cmd
			// The question names the issue by its number alone, which
			// another repository has too.
			if found && it.Number == number && g.Repo.Same(repo) {
				_, applies, refused = stateVerb(g, it, state)
			}
			switch {
			case refused != nil:
				return refused
			case !applies:
				return ui.Notify(toast.Info, ui.Meanwhile(n))
			}
			var op *optimistic.Op
			if state == core.StateClosed {
				op = svc.Close(g.Repo, number)
			} else {
				op = svc.Reopen(g.Repo, number)
			}
			return send(op, strings.ToLower(verb)+" "+n)
		},
	}, true, nil
}

// setState closes or reopens the selected issue, once the user confirms it
// in a modal of its own. The list shows the change from the cache at once,
// then it is sent.
func (s *Section) setState(state core.State) tea.Cmd {
	it, ok := s.target()
	if !ok {
		return nil
	}
	c, ok, refusal := stateChange(s.svc, s.gate(), it, state,
		func() (core.Issue, ui.Gate, bool) {
			it, ok := s.target()
			return it, s.gate(), ok
		},
		func(op *optimistic.Op, what string) tea.Cmd {
			about := core.Target{Repo: s.repo, Number: it.Number}.String()
			return tea.Batch(s.reload(), ui.Do(s.ctx, ui.IssuesTitle, ui.About(about, op), what))
		})
	if !ok {
		return refusal
	}
	return ui.OpenModal(ui.NewConfirmModal(c, s.keys.confirm, s.icons))
}

// gate decides what the viewer may do in the repository of the list.
func (s *Section) gate() ui.Gate {
	return ui.Gate{Repo: s.repo, Caps: s.caps, Viewer: s.viewer, Token: s.voice.Token, Icons: s.icons}
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

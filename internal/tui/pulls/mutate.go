package pulls

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// target returns the pull request the list keys act on: the selected one.
func (s *Section) target() (core.PullRequest, bool) {
	if s.feed == nil {
		return core.PullRequest{}, false
	}
	return s.feed.Selected()
}

// Which changes apply to pr.
func canMerge(pr core.PullRequest) bool  { return pr.State == core.StateOpen && !pr.Draft }
func canClose(pr core.PullRequest) bool  { return pr.State == core.StateOpen }
func canReopen(pr core.PullRequest) bool { return pr.State == core.StateClosed }
func canDraft(pr core.PullRequest) bool  { return pr.State == core.StateOpen }

// isChange reports whether msg is one of the change keys.
func (k keyMap) isChange(msg tea.KeyPressMsg) bool {
	return key.Matches(msg, k.Merge, k.Close, k.Reopen, k.ToggleDraft)
}

// action returns the change that msg asks of pr, if it applies to the
// state of pr. Close and reopen may share a key, which then toggles.
func (k keyMap) action(pr core.PullRequest, msg tea.KeyPressMsg) (ui.Action, bool) {
	switch {
	case key.Matches(msg, k.Merge) && pr.State == core.StateOpen:
		return ui.ActMerge, true
	case key.Matches(msg, k.Close) && canClose(pr):
		return ui.ActClose, true
	case key.Matches(msg, k.Reopen) && canReopen(pr):
		return ui.ActReopen, true
	case key.Matches(msg, k.ToggleDraft) && canDraft(pr):
		return ui.ActDraft, true
	}
	return 0, false
}

// change is a change that a key asks of a pull request: start shows it
// in the cache at once and returns the op that sends it, named by what.
// question asks the user to confirm it first, or is empty when it needs
// no confirmation.
type change struct {
	question, what string
	start          func() *optimistic.Op
}

// change returns the change that msg asks of pr, and ok when there is one.
// When the change doesn't apply, or g refuses it, ok is unset, and warn
// may explain why. A merge uses method, or else one the repository allows.
func (k keyMap) change(svc Service, g ui.Gate, method core.MergeMethod, pr core.PullRequest, msg tea.KeyPressMsg) (c change, ok bool, warn tea.Cmd) {
	a, ok := k.action(pr, msg)
	if !ok {
		return change{}, false, nil
	}
	if cmd, refused := g.Refuse(a, &pr.Issue); refused {
		return change{}, false, cmd
	}
	repo, number := g.Repo, pr.Number
	n := "#" + strconv.Itoa(number)
	switch a {
	case ui.ActMerge:
		if pr.Draft {
			return change{}, false, ui.Notify(toast.Warning, "Mark "+n+" ready for review before merging it.")
		}
		m, _ := g.Caps.MergeMethod(method)
		return change{
			question: mergeQuestion(pr, m),
			what:     "merge " + n,
			start:    func() *optimistic.Op { return svc.Merge(repo, number, m) },
		}, true, nil
	case ui.ActClose:
		return change{
			question: "Close PR " + n + "?",
			what:     "close " + n,
			start:    func() *optimistic.Op { return svc.Close(repo, number) },
		}, true, nil
	case ui.ActReopen:
		return change{
			question: "Reopen PR " + n + "?",
			what:     "reopen " + n,
			start:    func() *optimistic.Op { return svc.Reopen(repo, number) },
		}, true, nil
	case ui.ActDraft:
		if pr.Draft {
			return change{what: "mark " + n + " ready", start: func() *optimistic.Op { return svc.MarkReady(repo, number) }}, true, nil
		}
		return change{what: "convert " + n + " to draft", start: func() *optimistic.Op { return svc.ConvertToDraft(repo, number) }}, true, nil
	case ui.ActComment, ui.ActLabel, ui.ActRerun, ui.ActCancelRun:
	}
	return change{}, false, nil
}

// mergeQuestion asks to merge pr with method, such as "Squash-merge #79
// into main?". The screen shows the repository, so the question leaves
// it out, and it leads with the method, which a cut would lose.
func mergeQuestion(pr core.PullRequest, method core.MergeMethod) string {
	n := "#" + strconv.Itoa(pr.Number)
	var into string
	if pr.BaseRef != "" {
		into = " into " + pr.BaseRef
	}
	switch method {
	case core.MergeSquash:
		return "Squash-merge " + n + into + "?"
	case core.MergeRebase:
		return "Rebase-merge " + n + into + "?"
	case core.MergeCommit:
		return "Merge " + n + into + " with a merge commit?"
	}
	return "Merge " + n + into + "?"
}

// confirmed returns what makes the change that msg asked of pull request
// number, when the user says yes to question, or at once for a change that
// asks nothing. By then the pull request may have changed, such as merged
// elsewhere or retargeted, and so may what the viewer may do and how the
// repository merges, so it asks for the change again of the pull request
// and the gate that now returns. send sends that one only if it is still
// the change that question asked, and otherwise nothing.
func (k keyMap) confirmed(svc Service, method core.MergeMethod, number int, question string, msg tea.KeyPressMsg,
	now func() (core.PullRequest, ui.Gate, bool), send func(op *optimistic.Op, what string) tea.Cmd,
) func() tea.Cmd {
	return func() tea.Cmd {
		pr, g, found := now()
		var c change
		var ok bool
		var warn tea.Cmd
		if found {
			c, ok, warn = k.change(svc, g, method, pr, msg)
		}
		// The question names the pull request, so a different one, such as
		// the one under a cursor that moved, asks another question too.
		switch {
		case ok && c.question == question:
			return send(c.start(), c.what)
		case warn != nil:
			return warn
		}
		return ui.Notify(toast.Info, ui.Meanwhile("#"+strconv.Itoa(number)))
	}
}

// mutate starts the change that msg asks of the selected pull request, if
// it is one, once the user confirms it in a modal of its own. The change
// shows at once, and ui.Do sends it; the DoneMsg that follows shows the
// outcome.
func (s *Section) mutate(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if !s.keys.isChange(msg) {
		return nil, false
	}
	pr, ok := s.target()
	if !ok {
		return nil, true
	}
	c, ok, warn := s.keys.change(s.svc, s.gate(), s.mergeMethod, pr, msg)
	if !ok {
		return warn, true
	}
	run := s.keys.confirmed(s.svc, s.mergeMethod, pr.Number, c.question, msg,
		func() (core.PullRequest, ui.Gate, bool) {
			pr, ok := s.target()
			return pr, s.gate(), ok
		},
		func(op *optimistic.Op, what string) tea.Cmd {
			return tea.Batch(s.reload(), ui.Do(s.ctx, ui.PullsTitle, op, what))
		})
	if c.question == "" {
		return run(), true
	}
	return ui.OpenModal(ui.NewConfirmModal(ui.Confirm{Question: c.question, Run: run})), true
}

// reload shows the list again through the cache, which a change has just
// updated or rolled back.
func (s *Section) reload() tea.Cmd {
	if s.feed == nil {
		return nil
	}
	return s.feed.Reload()
}

// changeHelp returns the change keys, enabled when they apply to pr, which
// ok says there is, and g allows them. The merge key names the method
// when the repository refuses method, the configured one.
func (k keyMap) changeHelp(g ui.Gate, method core.MergeMethod, pr core.PullRequest, ok bool) []key.Binding {
	merge, closing, reopen, draft := k.Merge, k.Close, k.Reopen, k.ToggleDraft
	merge.SetEnabled(merge.Enabled() && ok && canMerge(pr))
	closing.SetEnabled(closing.Enabled() && ok && canClose(pr))
	reopen.SetEnabled(reopen.Enabled() && ok && canReopen(pr))
	draft.SetEnabled(draft.Enabled() && ok && canDraft(pr))
	if pr.Draft {
		draft.SetHelp(draft.Help().Key, "mark ready")
	}
	if m, allowed := g.Caps.MergeMethod(method); allowed && m != method {
		merge.SetHelp(merge.Help().Key, "merge ("+string(m)+")")
	}
	it := &pr.Issue
	return []key.Binding{
		g.Gated(merge, ui.ActMerge, it), g.Gated(closing, ui.ActClose, it),
		g.Gated(reopen, ui.ActReopen, it), g.Gated(draft, ui.ActDraft, it),
	}
}

// gate decides what the viewer may do in the repository of the list.
func (s *Section) gate() ui.Gate {
	return ui.Gate{Repo: s.repo, Caps: s.caps}
}

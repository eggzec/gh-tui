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

// change starts the change that msg asks of pr: the service shows it in
// the cache at once and returns the op that sends it, named by what. When
// the change doesn't apply, or g refuses it, op is nil, and warn may
// explain why. A merge uses method, or else one the repository allows.
func (k keyMap) change(svc Service, g ui.Gate, method core.MergeMethod, pr core.PullRequest, msg tea.KeyPressMsg) (op *optimistic.Op, what string, warn tea.Cmd) {
	a, ok := k.action(pr, msg)
	if !ok {
		return nil, "", nil
	}
	if cmd, refused := g.Refuse(a, &pr.Issue); refused {
		return nil, "", cmd
	}
	n := "#" + strconv.Itoa(pr.Number)
	switch a {
	case ui.ActMerge:
		if pr.Draft {
			return nil, "", ui.Notify(toast.Warning, "Mark "+n+" ready for review before merging it.")
		}
		m, _ := g.Caps.MergeMethod(method)
		return svc.Merge(g.Repo, pr.Number, m), "merge " + n, nil
	case ui.ActClose:
		return svc.Close(g.Repo, pr.Number), "close " + n, nil
	case ui.ActReopen:
		return svc.Reopen(g.Repo, pr.Number), "reopen " + n, nil
	case ui.ActDraft:
		if pr.Draft {
			return svc.MarkReady(g.Repo, pr.Number), "mark " + n + " ready", nil
		}
		return svc.ConvertToDraft(g.Repo, pr.Number), "convert " + n + " to draft", nil
	case ui.ActComment, ui.ActLabel, ui.ActRerun, ui.ActCancelRun:
	}
	return nil, "", nil
}

// mutate starts the change that msg asks of the selected pull request, if
// it is one. The change shows at once, and ui.Do sends it; the DoneMsg
// that follows shows the outcome.
func (s *Section) mutate(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if !s.keys.isChange(msg) {
		return nil, false
	}
	pr, ok := s.target()
	if !ok {
		return nil, true
	}
	op, what, warn := s.keys.change(s.svc, s.gate(), s.mergeMethod, pr, msg)
	if op == nil {
		return warn, true
	}
	return tea.Batch(s.reload(), ui.Do(s.ctx, ui.PullsTitle, op, what)), true
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

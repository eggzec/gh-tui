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

// target returns the pull request the keys act on: the open one, or the
// one selected in the list.
func (s *Section) target() (core.PullRequest, bool) {
	switch {
	case s.thread != nil:
		return s.detail.PullRequest, true
	case s.feed != nil:
		return s.feed.Selected()
	}
	return core.PullRequest{}, false
}

// Which changes apply to pr.
func canMerge(pr core.PullRequest) bool  { return pr.State == core.StateOpen && !pr.Draft }
func canClose(pr core.PullRequest) bool  { return pr.State == core.StateOpen }
func canReopen(pr core.PullRequest) bool { return pr.State == core.StateClosed }
func canDraft(pr core.PullRequest) bool  { return pr.State == core.StateOpen }

// mutate starts the change that msg asks for, if it is one. The change shows
// at once, and ui.Do sends it; the DoneMsg that follows shows the outcome.
func (s *Section) mutate(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := s.keys
	if !key.Matches(msg, k.Merge, k.Close, k.Reopen, k.ToggleDraft) {
		return nil, false
	}
	pr, ok := s.target()
	if !ok {
		return nil, true
	}
	n := "#" + strconv.Itoa(pr.Number)
	var (
		op   *optimistic.Op
		what string
	)
	switch {
	case key.Matches(msg, k.Merge) && pr.Draft && pr.State == core.StateOpen:
		return ui.Notify(toast.Warning, "Mark "+n+" ready for review before merging it."), true
	case key.Matches(msg, k.Merge) && canMerge(pr):
		op, what = s.svc.Merge(s.repo, pr.Number, s.mergeMethod), "merge "+n
	// Close and reopen may share a key, which then toggles.
	case key.Matches(msg, k.Close) && canClose(pr):
		op, what = s.svc.Close(s.repo, pr.Number), "close "+n
	case key.Matches(msg, k.Reopen) && canReopen(pr):
		op, what = s.svc.Reopen(s.repo, pr.Number), "reopen "+n
	case key.Matches(msg, k.ToggleDraft) && canDraft(pr) && pr.Draft:
		op, what = s.svc.MarkReady(s.repo, pr.Number), "mark "+n+" ready"
	case key.Matches(msg, k.ToggleDraft) && canDraft(pr):
		op, what = s.svc.ConvertToDraft(s.repo, pr.Number), "convert "+n+" to draft"
	default:
		return nil, true
	}
	return tea.Batch(s.reload(), ui.Do(s.ctx, op, what)), true
}

// reload shows the cache again, which a change has just updated or rolled
// back: the list is fetched through it and the open detail is read from it.
func (s *Section) reload() tea.Cmd {
	var cmds []tea.Cmd
	if s.feed != nil {
		cmds = append(cmds, s.feed.Reload())
	}
	if s.thread != nil {
		if d, ok := s.svc.CachedGet(s.repo, s.detail.Number); ok {
			s.detail = d
			cmds = append(cmds, s.showDetail())
		}
	}
	return tea.Batch(cmds...)
}

// mutationHelp returns the change keys, enabled when they apply to the
// target.
func (s *Section) mutationHelp() []key.Binding {
	k := s.keys
	pr, ok := s.target()
	merge, closing, reopen, draft := k.Merge, k.Close, k.Reopen, k.ToggleDraft
	merge.SetEnabled(merge.Enabled() && ok && canMerge(pr))
	closing.SetEnabled(closing.Enabled() && ok && canClose(pr))
	reopen.SetEnabled(reopen.Enabled() && ok && canReopen(pr))
	draft.SetEnabled(draft.Enabled() && ok && canDraft(pr))
	if pr.Draft {
		draft.SetHelp(draft.Help().Key, "mark ready")
	}
	return []key.Binding{merge, closing, reopen, draft}
}

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

// change starts the change that msg asks of pr in repo: the service shows
// it in the cache at once and returns the op that sends it, named by what.
// When the change doesn't apply, op is nil, and warn may explain why.
func (k keyMap) change(svc Service, method core.MergeMethod, repo core.RepoRef, pr core.PullRequest, msg tea.KeyPressMsg) (op *optimistic.Op, what string, warn tea.Cmd) {
	n := "#" + strconv.Itoa(pr.Number)
	switch {
	case key.Matches(msg, k.Merge) && pr.Draft && pr.State == core.StateOpen:
		return nil, "", ui.Notify(toast.Warning, "Mark "+n+" ready for review before merging it.")
	case key.Matches(msg, k.Merge) && canMerge(pr):
		return svc.Merge(repo, pr.Number, method), "merge " + n, nil
	// Close and reopen may share a key, which then toggles.
	case key.Matches(msg, k.Close) && canClose(pr):
		return svc.Close(repo, pr.Number), "close " + n, nil
	case key.Matches(msg, k.Reopen) && canReopen(pr):
		return svc.Reopen(repo, pr.Number), "reopen " + n, nil
	case key.Matches(msg, k.ToggleDraft) && canDraft(pr) && pr.Draft:
		return svc.MarkReady(repo, pr.Number), "mark " + n + " ready", nil
	case key.Matches(msg, k.ToggleDraft) && canDraft(pr):
		return svc.ConvertToDraft(repo, pr.Number), "convert " + n + " to draft", nil
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
	op, what, warn := s.keys.change(s.svc, s.mergeMethod, s.repo, pr, msg)
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
// ok says there is.
func (k keyMap) changeHelp(pr core.PullRequest, ok bool) []key.Binding {
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

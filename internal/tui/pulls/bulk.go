package pulls

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// isBulk reports whether msg is a change key that acts on every marked
// pull request while some are marked. Merge isn't: it pins the commit and
// the method of one pull request, so with marks it merges the one under
// the cursor, as without them, and its question names that one.
func (s *Section) isBulk(msg tea.KeyPressMsg) bool {
	k := s.keys
	return s.feed != nil && s.feed.Marks() > 0 && !keymap.Matches(msg, k.Merge) &&
		keymap.Matches(msg, k.Close, k.Reopen, k.ToggleDraft)
}

// bulkAction returns the change that msg asks of the marked pull
// requests. Close and reopen may share a key, which then follows the
// marked pull request under the cursor, or else the first marked one, and the draft key goes the way that
// pull request does when it is marked, and else the way of the first
// marked open one: to ready for review if that is a draft, and to a
// draft if it isn't. ready says which way the draft key goes.
func (s *Section) bulkAction(msg tea.KeyPressMsg, marked []core.PullRequest) (a ui.Action, ready bool) {
	k := s.keys
	under, hasUnder := s.feed.Selected()
	hasUnder = hasUnder && s.feed.Marked(under)
	if keymap.Matches(msg, k.Close, k.Reopen) {
		// A shared key goes the way of the marked row under the cursor,
		// or else of the first marked one.
		first := under
		if !hasUnder && len(marked) > 0 {
			first = marked[0]
		}
		closes, reopens := keymap.Matches(msg, k.Close), keymap.Matches(msg, k.Reopen)
		if closes && (!reopens || first.State != core.StateClosed) {
			return ui.ActClose, false
		}
		return ui.ActReopen, false
	}
	if hasUnder && under.State == core.StateOpen {
		return ui.ActDraft, under.Draft
	}
	for i := range marked {
		if pr := &marked[i]; pr.State == core.StateOpen {
			return ui.ActDraft, pr.Draft
		}
	}
	return ui.ActDraft, false
}

// bulkPlan returns what the change that msg asks does to the marked pull
// requests that are loaded: those it applies to, and those it leaves out,
// each with its reason.
func (s *Section) bulkPlan(msg tea.KeyPressMsg) ui.BulkPlan {
	marked, unloaded := s.feed.MarkedItems()
	a, ready := s.bulkAction(msg, marked)
	var p ui.BulkPlan
	switch {
	case a == ui.ActClose:
		p = ui.NewBulkPlan("Close", "Closed", "pull request", "", unloaded)
	case a == ui.ActReopen:
		p = ui.NewBulkPlan("Reopen", "Reopened", "pull request", "", unloaded)
	case ready:
		p = ui.NewBulkPlan("Mark", "Marked", "pull request", " ready for review", unloaded)
	default:
		p = ui.NewBulkPlan("Convert", "Converted", "pull request", " to draft", unloaded)
	}
	p.List = s.feed.ID()
	g, svc, repo := s.gate(), s.svc, s.repo
	for i := range marked {
		pr := &marked[i]
		if reason := bulkSkip(a, ready, pr); reason != "" {
			p.Skip(reason)
			continue
		}
		if ok, why := g.Allow(a, &pr.Issue); !ok {
			p.SkipRefused(why)
			continue
		}
		number := pr.Number
		n := "#" + strconv.Itoa(number)
		it := ui.BulkItem{Key: pullKey(*pr), Name: n}
		switch {
		case a == ui.ActClose:
			it.What, it.Start = "close "+n, func() ui.Op { return svc.Close(repo, number) }
		case a == ui.ActReopen:
			it.What, it.Start = "reopen "+n, func() ui.Op { return svc.Reopen(repo, number) }
		case ready:
			it.What, it.Start = "mark "+n+" ready", func() ui.Op { return svc.MarkReady(repo, number) }
		default:
			it.What, it.Start = "convert "+n+" to draft", func() ui.Op { return svc.ConvertToDraft(repo, number) }
		}
		p.Items = append(p.Items, it)
	}
	return p
}

// bulkSkip says why the change a asks, going the way ready says for the
// draft key, leaves pr out, or "" when it applies to it.
func bulkSkip(a ui.Action, ready bool, pr *core.PullRequest) string {
	switch {
	case a == ui.ActClose && pr.State == core.StateClosed:
		return "already closed"
	case a == ui.ActClose && pr.State == core.StateMerged:
		return "already merged"
	case a == ui.ActReopen && pr.State == core.StateOpen:
		return "already open"
	case a == ui.ActReopen && pr.State == core.StateMerged:
		return "merged"
	case a == ui.ActDraft && pr.State != core.StateOpen:
		return "not open"
	case a == ui.ActDraft && ready && !pr.Draft:
		return "already ready"
	case a == ui.ActDraft && !ready && pr.Draft:
		return "already draft"
	}
	return ""
}

// bulk asks to make the change that msg asks on every marked pull request
// it applies to, in one question that says how many it leaves out. The
// changes show at once, and the plan's Do sends them; the message that follows
// shows how they went.
func (s *Section) bulk(msg tea.KeyPressMsg) tea.Cmd {
	p := s.bulkPlan(msg)
	if p.Empty() {
		return ui.Notify(toast.Warning, p.Nothing())
	}
	repo := s.repo
	c := p.Confirm(
		func() (ui.BulkPlan, bool) {
			if s.feed == nil || !s.hasRepo || !s.repo.Same(repo) {
				return ui.BulkPlan{}, false
			}
			return s.bulkPlan(msg), true
		},
		func(p ui.BulkPlan) tea.Cmd {
			send := p.Do(s.ctx, ui.PullsTitle)
			return tea.Batch(s.reload(), send)
		})
	return ui.OpenModal(ui.NewConfirmModal(c, s.keys.confirm, s.icons))
}

// bulkDone shows the pull requests again from the cache, which the
// changes have updated or rolled back, and unmarks those that changed. The
// marks of the pull requests whose change failed stay, to try again.
func (s *Section) bulkDone(msg ui.BulkDoneMsg) tea.Cmd {
	if msg.From != ui.PullsTitle || s.feed == nil {
		return nil
	}
	// The list may be another one by now, such as of another tab.
	if msg.Plan.List == s.feed.ID() {
		s.feed.SetMarks(msg.FailedKeys()...)
	}
	return s.reload()
}

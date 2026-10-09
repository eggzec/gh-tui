package issues

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// issueKey identifies an issue in the feed.
func issueKey(it core.Issue) string { return strconv.Itoa(it.Number) }

// bulkState returns the state that msg asks the marked issues to move to,
// if it is the close or the reopen key. They may share a key, which then
// follows the marked issue under the cursor, or else the first marked one.
func (s *Section) bulkState(msg tea.KeyPressMsg) (state core.State, ok bool) {
	k := s.keys
	closes, reopens := key.Matches(msg, k.Close), key.Matches(msg, k.Reopen)
	switch {
	case closes && reopens:
		// A shared key goes the way of the marked issue under the
		// cursor, or else of the first marked one.
		marked, _ := s.list.MarkedItems()
		first, under := s.list.Selected()
		if !under || !s.list.Marked(first) {
			if len(marked) == 0 {
				return core.StateClosed, true
			}
			first = marked[0]
		}
		if first.State != core.StateOpen {
			return core.StateOpen, true
		}
		return core.StateClosed, true
	case closes:
		return core.StateClosed, true
	case reopens:
		return core.StateOpen, true
	}
	return "", false
}

// bulkPlan returns what moving the marked issues that are loaded to state
// does: those it applies to, and those it leaves out, each with its reason.
func (s *Section) bulkPlan(state core.State) ui.BulkPlan {
	marked, unloaded := s.list.MarkedItems()
	p := ui.NewBulkPlan("Close", "Closed", "issue", "", unloaded)
	a, from := ui.ActClose, core.StateOpen
	if state == core.StateOpen {
		p = ui.NewBulkPlan("Reopen", "Reopened", "issue", "", unloaded)
		a, from = ui.ActReopen, core.StateClosed
	}
	p.List = s.list.ID()
	g, svc, repo := s.gate(), s.svc, s.repo
	for i := range marked {
		it := &marked[i]
		if it.State != from {
			if from == core.StateOpen {
				p.Skip("already closed")
			} else {
				p.Skip("already open")
			}
			continue
		}
		if ok, why := g.Allow(a, it); !ok {
			p.SkipRefused(why)
			continue
		}
		number := it.Number
		n := "#" + strconv.Itoa(number)
		item := ui.BulkItem{Key: issueKey(*it), Name: n}
		if state == core.StateClosed {
			item.What, item.Start = "close "+n, func() ui.Op { return svc.Close(repo, number) }
		} else {
			item.What, item.Start = "reopen "+n, func() ui.Op { return svc.Reopen(repo, number) }
		}
		p.Items = append(p.Items, item)
	}
	return p
}

// bulk asks to close or reopen every marked issue it applies to, in one
// question that says how many it leaves out. The changes show at once,
// and the plan's Do sends them; the message that follows shows how they
// went. The labels key stays with the issue under the cursor: it opens a
// prompt about one issue.
func (s *Section) bulk(state core.State) tea.Cmd {
	p := s.bulkPlan(state)
	if p.Empty() {
		return ui.Notify(toast.Warning, p.Nothing())
	}
	repo := s.repo
	c := p.Confirm(
		func() (ui.BulkPlan, bool) {
			if !s.hasRepo || !s.repo.Same(repo) {
				return ui.BulkPlan{}, false
			}
			return s.bulkPlan(state), true
		},
		func(p ui.BulkPlan) tea.Cmd {
			send := p.Do(s.ctx, ui.IssuesTitle)
			return tea.Batch(s.reload(), send)
		})
	return ui.OpenModal(ui.NewConfirmModal(c, s.keys.confirm, s.icons))
}

// bulkDone shows the issues again from the cache, which the changes have
// updated or rolled back, and unmarks those that changed. The marks of the
// issues whose change failed stay, to try again.
func (s *Section) bulkDone(msg ui.BulkDoneMsg) tea.Cmd {
	if msg.From != ui.IssuesTitle {
		return nil
	}
	// The list may be another one by now, such as of another tab.
	if msg.Plan.List == s.list.ID() {
		s.list.SetMarks(msg.FailedKeys()...)
	}
	return s.reload()
}

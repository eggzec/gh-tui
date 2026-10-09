package notifications

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// bulkPlan returns what marking the marked threads that are loaded read,
// or done, does: those it applies to, and those it leaves out, each with
// its reason. Only the unread ones are marked read, and every one can be
// marked done.
func (s *Section) bulkPlan(read bool) ui.BulkPlan {
	marked, unloaded := s.feed.MarkedItems()
	tail := " as done"
	if read {
		tail = " as read"
	}
	p := ui.NewBulkPlan("Mark", "Marked", "notification", tail, unloaded)
	p.List = s.feed.ID()
	svc := s.svc
	for i := range marked {
		n := &marked[i]
		if read && !n.Unread {
			p.Skip("already read")
			continue
		}
		id, name := n.ID, threadName(*n)
		it := ui.BulkItem{Key: id, Name: name}
		if read {
			it.What, it.Start = "mark read "+name, func() ui.Op { return svc.MarkRead(id) }
		} else {
			it.What, it.Start = "mark done "+name, func() ui.Op { return svc.MarkDone(id) }
		}
		p.Items = append(p.Items, it)
	}
	return p
}

// bulk asks to mark every marked thread it applies to read, or done, in
// one question that says how many it leaves out. The changes show at
// once, and the plan's Do sends them; the message that follows shows how
// they went.
func (s *Section) bulk(read bool) tea.Cmd {
	p := s.bulkPlan(read)
	if p.Empty() {
		return ui.Notify(toast.Warning, p.Nothing())
	}
	c := p.Confirm(
		func() (ui.BulkPlan, bool) { return s.bulkPlan(read), true },
		func(p ui.BulkPlan) tea.Cmd {
			send := p.Do(s.ctx, ui.NotificationsTitle)
			return tea.Batch(s.reload(), send)
		})
	return ui.OpenModal(ui.NewConfirmModal(c, s.keys.confirm, s.icons))
}

// bulkDone shows the threads again from the cache, which the changes have
// updated or rolled back, and unmarks those that changed. The marks of the
// threads whose change failed stay, to try again.
func (s *Section) bulkDone(msg ui.BulkDoneMsg) tea.Cmd {
	if msg.From != ui.NotificationsTitle {
		return nil
	}
	if msg.Plan.List == s.feed.ID() {
		s.feed.SetMarks(msg.FailedKeys()...)
	}
	return s.reload()
}

package pulls

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// RequestReviews sends nothing.
func (f *fakeService) RequestReviews(core.RepoRef, int, []pulls.Reviewer) *optimistic.Op {
	return optimistic.New(func(context.Context) error { return nil })
}

// involvedService is a fake service whose cached timeline holds comments, and
// which records the reviews asked for.
type involvedService struct {
	*fakeService
	timeline []core.TimelineItem
	// asked is each request for review, in order, and refuse the error that
	// the Op of the next one returns.
	asked  [][]pulls.Reviewer
	refuse error
}

func (s *involvedService) RequestReviews(_ core.RepoRef, _ int, who []pulls.Reviewer) *optimistic.Op {
	s.asked = append(s.asked, who)
	if s.refuse != nil {
		return optimistic.Refused(s.refuse)
	}
	return s.fakeService.RequestReviews(core.RepoRef{}, 0, who)
}

func (s *involvedService) CachedTimeline(q pulls.TimelineQuery) (core.Page[core.TimelineItem], bool) {
	if q.Cursor != "" {
		return core.Page[core.TimelineItem]{}, false
	}
	return core.Page[core.TimelineItem]{Items: s.timeline}, true
}

func person(login, name string) core.User { return core.User{Login: login, Name: name} }

// involvedDetail has a requested reviewer, one who approved, an assignee, and
// a comment from the approver.
func involvedDetail(d core.PullRequestDetail) core.PullRequestDetail {
	d.Assignees = []core.User{person("bob", "Bob")}
	d.Reviewers = core.Reviewers{
		Requested:      []core.ReviewRequest{{User: person("rita", "Rita")}},
		RequestedTotal: 1,
		Latest:         []core.Verdict{{Author: person("dina", "Dina"), State: core.ReviewStateApproved}},
		LatestTotal:    1,
	}
	return d
}

// openedInvolved returns the modal of the first pull request with the
// involved pane focused and its people loaded.
func openedInvolved(t *testing.T, svc *involvedService) *panesModal {
	t.Helper()
	h := started(t, svc, 160, 30)
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the modal")
	}
	m.SetSize(160, 30)
	m.nav.SetFocus(paneInvolved)
	m.Update(panesDetailMsg{id: m.id, detail: involvedDetail(m.detail)})
	return m
}

// run feeds the messages that cmd makes back to the modal, until none is left,
// and returns the toasts among them.
func run(m *panesModal, cmd tea.Cmd) (toasts []string) {
	for range 8 {
		if cmd == nil {
			return toasts
		}
		msg := cmd()
		if n, ok := msg.(ui.NotifyMsg); ok {
			toasts = append(toasts, n.Text)
		}
		cmd = m.Update(msg)
	}
	return toasts
}

// moveTo presses down until the cursor is on the person login.
func moveTo(m *panesModal, login string) bool {
	for range 30 {
		if p, ok := m.inv.tree.Selected(); ok && p.Login == login {
			return true
		}
		m.Update(keyMsg("j"))
	}
	return false
}

var stamp = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

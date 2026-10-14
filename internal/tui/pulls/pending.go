package pulls

import (
	"log/slog"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The user's pending review on GitHub, such as one started on the web: its
// comments show in the tray beside the drafts, tagged as on GitHub, and a
// review submitted from here adds the drafts to it. They are read-only here,
// and are never written to disk.

// draftOnGitHub is what the diff and the list call a comment of the pending
// review on GitHub.
const draftOnGitHub = "on GitHub"

// onGitHubText is what the user reads when they try to change a comment that
// is in their pending review on GitHub.
const onGitHubText = "This comment is in your pending review on GitHub; change it there."

// pendingLoadedMsg carries the pending review read from GitHub to the modal
// that asked. ok is false when the user has none.
type pendingLoadedMsg struct {
	owner  *detailModal
	review core.PendingReview
	ok     bool
	err    error
}

// loadPending reads the user's pending review of the pull request.
func (m *detailModal) loadPending() tea.Cmd {
	svc, ctx, repo, n := m.svc, m.ctx, m.repo, m.number
	return func() tea.Msg {
		r, ok, err := svc.PendingReview(ctx, repo, n)
		return pendingLoadedMsg{owner: m, review: r, ok: ok, err: err}
	}
}

// receivePending takes the pending review that was read: its comments replace
// the ones of the last read among the drafts.
func (m *detailModal) receivePending(msg pendingLoadedMsg) tea.Cmd {
	if msg.owner != m {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() == nil {
			slog.WarnContext(m.ctx, "pending review not read", "span", "tui", "err", ui.ShortPath(msg.err.Error()))
		}
		return nil
	}
	var save tea.Cmd
	if r := m.tray.reconcile; r != nil {
		m.tray.reconcile = nil
		if m.leaveAdded(msg.review, r) {
			save = m.saveTray()
		}
	}
	m.tray.pending = core.PendingReview{}
	if msg.ok {
		m.tray.pending = msg.review
	}
	m.placePending()
	m.prefillSummary()
	return tea.Batch(save, m.showThreadChange())
}

// sameBody reports whether two comment texts are the same once their line
// endings and the space around them are set aside, as GitHub may change them.
func sameBody(a, b string) bool {
	norm := func(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }
	return norm(a) == norm(b)
}

// leaveAdded drops the drafts among those a failed review sent that it added
// to the pending review, which are the comments that it holds now and did not
// before that say what a draft does. It reports whether any went.
func (m *detailModal) leaveAdded(now core.PendingReview, r *reconcileState) bool {
	gone := false
	for _, c := range now.Comments {
		if slices.Contains(r.before, c.ID) {
			continue
		}
		i := slices.IndexFunc(m.tray.drafts, func(d tray.Draft) bool {
			return !d.OnGitHub && slices.Contains(r.sent, d.ID) && d.Path == c.Path && sameBody(d.Body, c.Body) &&
				(d.ThreadID != "") == (c.ReplyTo != "") && (d.ThreadID != "" || d.Line == c.Line || c.Line == 0)
		})
		if i >= 0 {
			m.tray.drafts = slices.Delete(m.tray.drafts, i, i+1)
			gone = true
		}
	}
	return gone
}

// dropPending forgets the pending review and its comments, once it is gone
// from GitHub.
func (m *detailModal) dropPending() {
	m.tray.pending = core.PendingReview{}
	m.tray.drafts = slices.DeleteFunc(m.tray.drafts, isOnGitHub)
	if len(m.tray.drafts) == 0 {
		m.tray.drafts = nil
	}
}

// isOnGitHub reports whether d is a comment of the pending review on GitHub.
func isOnGitHub(d tray.Draft) bool { return d.OnGitHub }

// placePending puts the comments of the pending review among the drafts, in
// place of those put there before. A comment says no side, so it takes the one
// of its thread, which the threads read tell: it is placed again as they come
// in.
func (m *detailModal) placePending() {
	m.tray.drafts = slices.DeleteFunc(m.tray.drafts, isOnGitHub)
	p := m.tray.pending
	for _, c := range p.Comments {
		d := tray.Draft{
			ID: c.ID, Commit: p.Commit, Path: c.Path, Side: tray.Right,
			Line: c.Line, StartLine: c.StartLine, Body: c.Body, Outdated: c.Outdated, OnGitHub: true,
		}
		if d.Line < 1 && !c.File {
			// The code changed since: the line it was on is what the user knows.
			d.Line, d.StartLine = c.OriginalLine, c.OriginalStartLine
		}
		if c.ReplyTo != "" {
			// The comment it answers is not always among the threads read.
			d.ThreadID = c.ReplyTo
			if th, ok := m.threadHolding(c.ReplyTo); ok {
				d.ThreadID = th.ID
			}
		}
		th, ok := m.threadHolding(c.ID)
		if !ok && d.ThreadID != "" {
			th, ok = m.threadByID(d.ThreadID)
		}
		if ok && th.Side == core.SideLeft {
			d.Side = tray.Left
		}
		m.tray.drafts = append(m.tray.drafts, d)
	}
}

// threadHolding returns the thread read that has comment id, among those shown
// and those that are the pending review's.
func (m *detailModal) threadHolding(id string) (*core.Thread, bool) {
	for _, ts := range [...][]core.Thread{m.th.all, m.th.hidden} {
		for i := range ts {
			if slices.ContainsFunc(ts[i].Comments, func(c core.ThreadComment) bool { return c.ID == id }) {
				return &ts[i], true
			}
		}
	}
	return nil, false
}

// splitPending separates the pending review's comments from the threads: shown
// are the threads without them, as the Conversation and the diff draw them,
// and hidden the pending part of each thread that has one, a thread of its own
// for a comment that starts one. Both keep the thread's ID, so that a comment
// of the review finds the thread it is in.
func splitPending(all []core.Thread) (shown, hidden []core.Thread) {
	shown = make([]core.Thread, 0, len(all))
	for i := range all {
		th := all[i]
		if th.Pending {
			hidden = append(hidden, th)
			continue
		}
		if !slices.ContainsFunc(th.Comments, func(c core.ThreadComment) bool { return c.Pending }) {
			shown = append(shown, th)
			continue
		}
		part := th
		part.Comments = slices.DeleteFunc(slices.Clone(th.Comments), func(c core.ThreadComment) bool { return !c.Pending })
		hidden = append(hidden, part)
		th.Comments = slices.DeleteFunc(slices.Clone(th.Comments), func(c core.ThreadComment) bool { return c.Pending })
		th.TotalComments = max(th.TotalComments-len(part.Comments), len(th.Comments))
		shown = append(shown, th)
	}
	return shown, hidden
}

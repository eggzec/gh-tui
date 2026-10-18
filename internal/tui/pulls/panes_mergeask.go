package pulls

import (
	"cmp"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// askMerge asks how to merge the pull request, as the list's merge key does,
// from whichever pane has the focus. The press that stands for the list's
// action is the one the list's keys take, whatever key the modal binds.
func (m *panesModal) askMerge() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, checking(m.icons, m.number))
	}
	s := m.list
	press := keymap.ActionPress("pulls.merge")
	c, ok, warn := s.keys.change(m.svc, m.gate(), mergeAs{method: s.mergeMethod}, m.detail, press)
	if !ok {
		if warn == nil {
			warn = ui.Notify(toast.Info, "Pull request #"+strconv.Itoa(m.number)+" isn't open.")
		}
		return warn
	}
	about := core.Target{Repo: m.repo, Number: m.number}.String()
	svc, ctx, repo, number := m.svc, m.ctx, m.repo, m.number
	draftKey := mergeDraftKey(s.draftScope, repo, number)
	// A merge the user confirmed goes out, or says why not, whether the modal
	// is still open or not: what it decides on is the stored detail.
	now := func() (core.PullRequestDetail, ui.Gate, bool) {
		if d, ok := svc.CachedGet(repo, number); ok {
			return d, m.gate(), true
		}
		return m.detail, m.gate(), m.loaded
	}
	send := func(c change, op *optimistic.Op) tea.Cmd {
		s.mergeMethod = cmp.Or(c.of.method, s.mergeMethod)
		var sent ui.Op = op
		if c.chooses {
			sent = forgetting(op, s.drafts, draftKey)
		}
		// The detail is read once GitHub has answered.
		do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(about, sent), c.what)
		return tea.Batch(s.reload(), tea.Sequence(do, m.revalidate()))
	}
	// What the read stored is what the next question asks of.
	refresh := func() {
		if d, ok := svc.CachedGet(repo, number); ok {
			m.detail = d
		}
	}
	// The read outlives the modal, and so does its owner: the list, which the
	// root reaches whether or not a modal is open.
	again := &rereader{owner: s, about: about, refresh: refresh, read: func() error {
		_, err := svc.Revalidate(m.sendCtx, repo, number)
		return err
	}}
	if c.chooses {
		d := newMergeDialog(mergeAsk{
			ctx: ctx, svc: svc, keys: s.keys, c: c, key: press, now: now, send: send, again: again,
			drafts: s.drafts, draftKey: draftKey, icons: m.icons, back: m,
		}, s.keys.mergeDialog)
		return tea.Sequence(ui.OpenModalOver(d, m), d.Init())
	}
	m.ask(s.keys.ask(svc, c, press, now, send, again))
	return nil
}

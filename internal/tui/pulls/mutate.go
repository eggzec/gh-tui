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
	return keymap.Matches(msg, k.Merge, k.Close, k.Reopen, k.ToggleDraft)
}

// action returns the change that msg asks of pr, if it applies to the
// state of pr. Close and reopen may share a key, which then toggles.
func (k keyMap) action(pr core.PullRequest, msg tea.KeyPressMsg) (ui.Action, bool) {
	switch {
	case keymap.Matches(msg, k.Merge) && pr.State == core.StateOpen:
		return ui.ActMerge, true
	case keymap.Matches(msg, k.Close) && canClose(pr):
		return ui.ActClose, true
	case keymap.Matches(msg, k.Reopen) && canReopen(pr):
		return ui.ActReopen, true
	case keymap.Matches(msg, k.ToggleDraft) && canDraft(pr):
		return ui.ActDraft, true
	}
	return 0, false
}

// change is a change that a key asks of a pull request: start shows it
// in the cache at once and returns the op that sends it, named by what.
// question asks the user to confirm it first, and of says what it is. A
// merge that the repository allows other methods for has next, which
// returns the same change with the next of them.
type change struct {
	question, what string
	of             changeOf
	start          func() *optimistic.Op
	next           func() change
}

// changeOf says what a change is: its action on pull request number of
// repo, for a merge what it does (see mergeKind), the method, the base
// branch and the head commit, and for the draft toggle which way it goes,
// so that a yes makes only the change it was asked about.
type changeOf struct {
	repo   core.RepoRef
	number int
	action ui.Action
	kind   mergeKind
	method core.MergeMethod
	base   string
	// head is the commit a merge is pinned to.
	head string
	// ready is set when the draft toggle marks a draft ready.
	ready bool
}

// same reports whether c and d are one change.
func (c changeOf) same(d changeOf) bool {
	return c.repo.Same(d.repo) && c.number == d.number && c.action == d.action &&
		c.kind == d.kind && c.method == d.method && c.base == d.base && c.head == d.head && c.ready == d.ready
}

// change returns the change that msg asks of d, and ok when there is one.
// When the change doesn't apply, or g refuses it, ok is unset, and warn
// may explain why. A merge uses method, or else one the repository allows.
func (k keyMap) change(svc Service, g ui.Gate, method core.MergeMethod, d core.PullRequestDetail, msg tea.KeyPressMsg) (c change, ok bool, warn tea.Cmd) {
	pr := d.PullRequest
	a, ok := k.action(pr, msg)
	if !ok {
		return change{}, false, nil
	}
	if cmd, refused := g.Refuse(a, &pr.Issue); refused {
		return change{}, false, cmd
	}
	repo, number := g.Repo, pr.Number
	n := "#" + strconv.Itoa(number)
	of := changeOf{repo: repo, number: number, action: a}
	switch a {
	case ui.ActMerge:
		return k.mergeChange(svc, g, method, d, of)
	case ui.ActClose:
		return change{
			of:       of,
			question: "Close PR " + n + "?",
			what:     "close " + n,
			start:    func() *optimistic.Op { return svc.Close(repo, number) },
		}, true, nil
	case ui.ActReopen:
		return change{
			of:       of,
			question: "Reopen PR " + n + "?",
			what:     "reopen " + n,
			start:    func() *optimistic.Op { return svc.Reopen(repo, number) },
		}, true, nil
	case ui.ActDraft:
		of.ready = pr.Draft
		if pr.Draft {
			return change{
				of:       of,
				question: "Mark PR " + n + " ready for review?",
				what:     "mark " + n + " ready",
				start:    func() *optimistic.Op { return svc.MarkReady(repo, number) },
			}, true, nil
		}
		return change{
			of:       of,
			question: "Convert PR " + n + " to a draft?",
			what:     "convert " + n + " to draft",
			start:    func() *optimistic.Op { return svc.ConvertToDraft(repo, number) },
		}, true, nil
	case ui.ActComment, ui.ActLabel, ui.ActRerun, ui.ActCancelRun, ui.ActMarkRead, ui.ActStar:
	}
	return change{}, false, nil
}

// confirmed returns what makes asked, the change that msg asked of a pull
// request, when the user says yes to its question. By then the pull
// request may have changed, such as merged elsewhere or retargeted, and
// so may what the viewer may do, how the repository merges and even which
// repository the list shows, so it asks for the change again of the pull
// request and the gate that now return. send sends that one only if it is
// still the change asked, the same action with the same method, base and
// head on the same pull request of the same repository, and otherwise
// nothing.
//
// A merge reads the detail again before it asks again, so that what it
// decides on is as current as GitHub's answer: what said the pull request
// could merge a minute ago may not hold. again says how.
func (k keyMap) confirmed(svc Service, asked change, msg tea.KeyPressMsg,
	now func() (core.PullRequestDetail, ui.Gate, bool), send func(c change, op *optimistic.Op) tea.Cmd,
	again *rereader,
) func() tea.Cmd {
	decide := func() tea.Cmd {
		d, g, found := now()
		var c change
		var ok bool
		var warn tea.Cmd
		if found {
			// A merge asks again for the method it was asked with, which
			// the user may have changed.
			c, ok, warn = k.change(svc, g, asked.of.method, d, msg)
		}
		switch {
		case ok && c.of.same(asked.of):
			return send(c, c.start())
		case warn != nil:
			return warn
		}
		return ui.Notify(toast.Info, ui.Meanwhile("#"+strconv.Itoa(asked.of.number)))
	}
	if again == nil || asked.of.action != ui.ActMerge {
		return decide
	}
	return func() tea.Cmd {
		return func() tea.Msg {
			err := again.read()
			return resendMsg{owner: again.owner, run: func() tea.Cmd {
				if err != nil {
					return ui.Fail("check #"+strconv.Itoa(asked.of.number), core.About(again.about, err))
				}
				again.refresh()
				return decide()
			}}
		}
	}
}

// rereader reads the detail of a pull request again for a merge that was
// confirmed, which owner, the section or modal that asked, then goes on
// with, in its Update. read does the request, and refresh shows what it
// read to the owner. about names the pull request in a failure.
type rereader struct {
	owner   any
	read    func() error
	refresh func()
	about   string
}

// resendMsg carries the read that a confirmed merge waited for, to its
// owner, which runs what is left.
type resendMsg struct {
	owner any
	run   func() tea.Cmd
}

// ask returns the question that confirms c, which sends it once the user
// says yes. A merge that has other methods steps through them with the
// key for it.
func (k keyMap) ask(svc Service, c change, msg tea.KeyPressMsg,
	now func() (core.PullRequestDetail, ui.Gate, bool), send func(c change, op *optimistic.Op) tea.Cmd,
	again *rereader,
) ui.Confirm {
	q := ui.Confirm{Question: c.question, Run: k.confirmed(svc, c, msg, now, send, again)}
	if c.next != nil {
		q.Cycle = func() ui.Confirm { return k.ask(svc, c.next(), msg, now, send, again) }
	}
	return q
}

// mutate starts the change that msg asks of the selected pull request, if
// it is one, once the user confirms it in a modal of its own. The change
// shows at once, and ui.Do sends it; the DoneMsg that follows shows the
// outcome. A merge of a pull request whose detail hasn't been read waits
// for it, which says whether it may merge and how.
func (s *Section) mutate(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if !s.keys.isChange(msg) || s.feed != nil && s.feed.Takes(msg) {
		return nil, false
	}
	if s.isBulk(msg) {
		return s.bulk(msg), true
	}
	pr, ok := s.target()
	if !ok {
		return nil, true
	}
	// What may merge, and how, is read again when the key is pressed, as
	// the modal reads it again when it opens: it changes without the pull
	// request's update time moving.
	if a, ok := s.keys.action(pr, msg); ok && a == ui.ActMerge && !pr.Draft {
		if s.checking == pr.Number {
			return nil, true
		}
		return s.checkMerge(pr, msg), true
	}
	return s.confirm(pr, msg), true
}

// confirm asks the user to confirm the change that msg asks of pr, in a
// modal of its own, or says why there is none.
func (s *Section) confirm(pr core.PullRequest, msg tea.KeyPressMsg) tea.Cmd {
	c, ok, warn := s.keys.change(s.svc, s.gate(), s.mergeMethod, s.subject(pr), msg)
	if !ok {
		return warn
	}
	about := core.Target{Repo: s.repo, Number: pr.Number}.String()
	svc, ctx, repo, number := s.svc, s.ctx, s.repo, pr.Number
	q := s.keys.ask(s.svc, c, msg,
		func() (core.PullRequestDetail, ui.Gate, bool) {
			pr, ok := s.target()
			return s.subject(pr), s.gate(), ok
		},
		func(c change, op *optimistic.Op) tea.Cmd {
			s.mergeMethod = cmp.Or(c.of.method, s.mergeMethod)
			return tea.Batch(s.reload(), ui.Do(s.ctx, ui.PullsTitle, ui.About(about, op), c.what))
		},
		// The row shows what the read stored, which subject reads.
		&rereader{owner: s, about: about, refresh: func() {}, read: func() error {
			_, err := svc.Revalidate(ctx, repo, number)
			return err
		}})
	return ui.OpenModal(ui.NewConfirmModal(q, s.keys.confirm, s.icons))
}

// subject returns pr, a row of the list, with what is cached of its
// detail: whether it may merge, and how.
func (s *Section) subject(pr core.PullRequest) core.PullRequestDetail {
	d := core.PullRequestDetail{PullRequest: pr}
	if c, ok := s.svc.CachedGet(s.repo, pr.Number); ok {
		// What the detail says of the reviews and checks is the later read.
		d.Merge, d.ReviewDecision, d.Checks = c.Merge, c.ReviewDecision, c.Checks
	}
	return d
}

// mergeReadMsg carries the detail of pull request number of repo, which a
// merge key waited for, with the key.
type mergeReadMsg struct {
	repo   core.RepoRef
	number int
	key    tea.KeyPressMsg
	err    error
}

// checkMerge reads the detail of pr again, which says whether it may merge,
// and then goes on with the merge that msg asks of it.
func (s *Section) checkMerge(pr core.PullRequest, msg tea.KeyPressMsg) tea.Cmd {
	svc, ctx, repo, number := s.svc, s.ctx, s.repo, pr.Number
	s.checking = number
	read := func() tea.Msg {
		_, err := svc.Revalidate(ctx, repo, number)
		return mergeReadMsg{repo: repo, number: number, key: msg, err: err}
	}
	return tea.Batch(ui.Notify(toast.Info, "Checking #"+strconv.Itoa(number)+"…"), read)
}

// merged goes on with the merge that waited for the detail of its pull
// request, unless the user moved on since.
func (s *Section) merged(msg mergeReadMsg) tea.Cmd {
	s.checking = 0
	pr, ok := s.target()
	if !s.hasRepo || !msg.repo.Same(s.repo) || !ok || pr.Number != msg.number {
		return nil
	}
	if msg.err != nil {
		return ui.Fail("check #"+strconv.Itoa(msg.number), core.About(core.Target{Repo: msg.repo, Number: msg.number}.String(), msg.err))
	}
	return s.confirm(pr, msg.key)
}

// reload shows the list again through the cache, which a change has just
// updated or rolled back.
func (s *Section) reload() tea.Cmd {
	if s.feed == nil {
		return nil
	}
	return s.feed.Reload()
}

// withChanges returns k with the change keys enabled when they apply to
// pr, which ok says there is, and g allows them. The merge key names the
// method when the repository refuses method, the preferred one.
func (k keyMap) withChanges(g ui.Gate, method core.MergeMethod, pr core.PullRequest, ok bool) keyMap {
	k.Merge.SetEnabled(k.Merge.Enabled() && ok && canMerge(pr))
	k.Close.SetEnabled(k.Close.Enabled() && ok && canClose(pr))
	k.Reopen.SetEnabled(k.Reopen.Enabled() && ok && canReopen(pr))
	k.ToggleDraft.SetEnabled(k.ToggleDraft.Enabled() && ok && canDraft(pr))
	if pr.Draft {
		k.ToggleDraft.SetHelp(k.ToggleDraft.Help().Key, "mark ready")
	}
	if m, allowed := g.Caps.MergeMethod(method); allowed && method != "" && m != method {
		k.Merge.SetHelp(k.Merge.Help().Key, "merge ("+string(m)+")")
	}
	it := &pr.Issue
	k.Merge, k.Close = g.Gated(k.Merge, ui.ActMerge, it), g.Gated(k.Close, ui.ActClose, it)
	k.Reopen, k.ToggleDraft = g.Gated(k.Reopen, ui.ActReopen, it), g.Gated(k.ToggleDraft, ui.ActDraft, it)
	return k
}

// gate decides what the viewer may do in the repository of the list.
func (s *Section) gate() ui.Gate {
	return ui.Gate{Repo: s.repo, Caps: s.caps, Token: s.voice.Token, Icons: s.icons}
}

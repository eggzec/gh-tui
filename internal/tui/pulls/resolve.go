package pulls

import (
	"slices"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// threadResolvedMsg reports that GitHub answered the change of review thread
// id, which resolved it, or opened it again if resolved is false. err is set
// when it refused, and the cache was rolled back.
type threadResolvedMsg struct {
	thread   int64
	id       string
	resolved bool
	err      error
}

// resolveBinding returns the key that resolves or unresolves the thread under
// the cursor of the diff, enabled only while the cursor is on one, and named
// for what it would do.
func (m *detailModal) resolveBinding() key.Binding {
	b := m.keys.diffResolve
	th, ok := m.threadUnderCursor()
	b.SetEnabled(b.Enabled() && ok)
	if ok && th.Resolved {
		b.SetHelp(b.Help().Key, "unresolve thread")
	}
	return b
}

// pressResolve takes the key that resolves or unresolves a thread: the one
// under the cursor of the Conversation tab's block, or of the diff. It reports
// whether it took the key.
func (m *detailModal) pressResolve(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case m.onConversation() && keymap.Matches(msg, m.keys.resolve):
		t := &m.th
		i := t.cursorRow()
		if !t.threadsShow() || i >= len(t.list) || m.thread.OnDiagram() {
			return nil, false
		}
		return m.askResolve(t.list[i].ID), true
	case m.onFiles() && keymap.Matches(msg, m.keys.diffResolve):
		th, ok := m.threadUnderCursor()
		if !ok {
			return nil, false
		}
		return m.askResolve(th.ID), true
	}
	return nil, false
}

// askResolve asks whether to resolve thread id, or to open it again if it is
// resolved, unless the viewer may not or a change of it is under way.
func (m *detailModal) askResolve(id string) tea.Cmd {
	th, ok := m.threadByID(id)
	if !ok || m.th.pending[id] || !m.loaded {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActResolve, &m.detail.Issue); refused {
		return cmd
	}
	resolve := !th.Resolved
	question := "Resolve this thread?"
	if !resolve {
		question = "Unresolve this thread?"
	}
	if resolve && !th.ViewerCanResolve {
		return ui.Notify(toast.Info, "You can't resolve this thread.")
	}
	if !resolve && !th.ViewerCanUnresolve {
		return ui.Notify(toast.Info, "You can't unresolve this thread.")
	}
	m.ask = &ui.Confirm{Question: question, Run: func() tea.Cmd { return m.sendResolve(id, resolve) }}
	return nil
}

// sendResolve sends the change of thread id, and marks it as under way until
// GitHub answers. The thread itself changes when it does.
func (m *detailModal) sendResolve(id string, resolve bool) tea.Cmd {
	th, ok := m.threadByID(id)
	if !ok || th.Resolved == resolve || m.th.pending[id] {
		return ui.Notify(toast.Info, "The thread changed meanwhile.")
	}
	if m.th.pending == nil {
		m.th.pending = map[string]bool{}
	}
	m.th.pending[id] = true
	what := "unresolve a thread of #" + strconv.Itoa(m.number)
	if resolve {
		what = "resolve a thread of #" + strconv.Itoa(m.number)
	}
	op := m.svc.ResolveThread(m.repo, m.number, id, resolve)
	do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(m.subject(), op), what)
	from := m.thread.ID()
	return tea.Batch(m.showThreadChange(), func() tea.Msg {
		done, _ := do().(ui.DoneMsg)
		answer := func() tea.Msg { return threadResolvedMsg{thread: from, id: id, resolved: resolve, err: done.Err} }
		if done.Err == nil {
			return answer()
		}
		// The modal may be closed by now, and then nothing else would say
		// that the change was refused.
		return tea.BatchMsg{ui.Fail(what, done.Err), answer}
	})
}

// resolved takes GitHub's answer to a change of a thread. A refusal leaves
// the thread as it was; the command that sent it has shown why. Otherwise the
// thread leaves the unresolved ones, and folds in the diff, or comes back
// unfolded.
func (m *detailModal) resolved(msg threadResolvedMsg) tea.Cmd {
	if msg.thread != m.thread.ID() {
		return nil
	}
	delete(m.th.pending, msg.id)
	if msg.err != nil {
		// A read may have taken the change while it was under way.
		return tea.Batch(m.showThreadChange(), m.reload())
	}
	// The slice may be shared with a page that was kept.
	all := slices.Clone(m.th.all)
	if i := slices.IndexFunc(all, func(th core.Thread) bool { return th.ID == msg.id }); i >= 0 {
		all[i].Resolved = msg.resolved
		all[i].ResolvedBy = core.User{}
		all[i].ViewerCanResolve, all[i].ViewerCanUnresolve = !msg.resolved, msg.resolved
	}
	m.th.pin()
	m.th.all, m.th.list = all, unresolved(all)
	m.thread.Redraw()
	cmd := m.showThreadChange()
	if f := m.files; f != nil {
		f.diff.SetBlockFolded(msg.id, msg.resolved)
	}
	// The detail's counts moved with the change in the cache.
	return tea.Batch(m.reload(), cmd)
}

// showThreadChange draws the threads again, in the Conversation tab and in the
// diff.
func (m *detailModal) showThreadChange() tea.Cmd {
	m.applyThreadBlocks()
	return m.show()
}

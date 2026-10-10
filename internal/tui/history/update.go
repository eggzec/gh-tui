package history

import (
	"cmp"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// Update handles the modal's keys and its reads, and passes everything else
// to the graph, the pager and the filter. Messages of other modals, such as
// the one this replaced, are ignored.
func (m *Modal) Update(msg tea.Msg) tea.Cmd {
	return m.update(msg)
}

func (m *Modal) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.press(msg)
	case branchesMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receiveBranches(msg)
	case branchRestMsg:
		if msg.id != m.id {
			return nil
		}
		return m.compare(msg)
	case compareMsg:
		if msg.id == m.id {
			m.receiveCompare(msg)
		}
		return nil
	case ui.AheadMsg:
		return tea.Batch(m.ahead.Rested(msg), m.compares.Rested(msg))
	case restMsg:
		if msg.id != m.id {
			return nil
		}
		return m.rested(msg)
	case detailMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receiveDetail(msg)
	case ui.OnlineMsg:
		// A rate limit is the token's, and has lifted unless one holds.
		if !msg.Limited {
			m.ahead.Resume()
			m.compares.Resume()
		}
		return m.online()
	case filesMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receiveFiles(msg)
	case headMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receiveHead(msg)
	case graph.SelectMsg:
		if msg.ID != m.graph.model.ID() {
			return nil
		}
		c, ok := msg.Commit.Value.(core.Commit)
		if !ok {
			return nil
		}
		return m.follow(c)
	case graph.ChosenMsg:
		if msg.ID != m.graph.model.ID() {
			return nil
		}
		m.setFocus(commitPane)
		return m.loadDetail()
	case picker.ChosenMsg, picker.CancelMsg:
		return m.updateFilter(msg)
	case ui.SyncMsg:
		// The revalidator found that a branch moved, and cached it.
		if msg.Err != nil || msg.Key != historysvc.SyncKey(m.repo) {
			return nil
		}
		return tea.Batch(m.loadBranches("", false), m.readHead())
	case spinner.TickMsg:
		if msg.ID == m.spin.ID() {
			return m.tick(msg)
		}
	}
	// Ticks, pages and highlights of the bubbles inside.
	graphCmd, filterCmd := m.updateGraph(msg), m.updateFilter(msg)
	var cmd tea.Cmd
	m.commit.pager, cmd = m.commit.pager.Update(msg)
	return tea.Batch(graphCmd, filterCmd, cmd)
}

// tick spins the spinner while something loads.
func (m *Modal) tick(msg spinner.TickMsg) tea.Cmd {
	if !m.loading() {
		m.spinning = false
		return nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return cmd
}

// press handles a key: an open filter or pager search takes every key, the
// modal's own keys come next, and then those of the focused pane.
func (m *Modal) press(msg tea.KeyPressMsg) tea.Cmd {
	if m.branches.filter != nil && m.focus == branchPane {
		return m.updateFilter(msg)
	}
	patch := m.focus == commitPane && m.commit.patch
	if patch && m.commit.pager.Capturing() {
		return m.updatePager(msg)
	}
	switch {
	case keymap.Matches(msg, m.keys.Next):
		m.setFocus((m.focus + 1) % numPanes)
		return m.focused()
	case keymap.Matches(msg, m.keys.Prev):
		m.setFocus((m.focus + numPanes - 1) % numPanes)
		return m.focused()
	case m.keys.focusOf(msg) >= 0:
		m.setFocus(m.keys.focusOf(msg))
		return m.focused()
	case keymap.Matches(msg, m.keys.Open):
		return m.open()
	case keymap.Matches(msg, m.keys.ResetBase):
		if m.base.Ref == "" {
			return nil
		}
		return m.useBase(ui.BaseMsg{Repo: m.repo})
	case !m.narrow() && keymap.Matches(msg, m.keys.Zoom):
		m.zoom = !m.zoom
		m.layout()
		return nil
	case keymap.Matches(msg, m.keys.Dismiss):
		return m.dismiss()
	case keymap.Matches(msg, m.keys.Back):
		return m.back()
	case patch:
		return m.updatePager(msg)
	case keymap.Matches(msg, m.keys.base(m.focus)):
		return m.useSelected()
	}
	switch m.focus {
	case branchPane:
		return m.pressBranches(msg)
	case graphPane:
		return m.updateGraph(msg)
	default:
		return m.pressCommit(msg)
	}
}

func (m *Modal) updatePager(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.commit.pager, cmd = m.commit.pager.Update(msg)
	return cmd
}

// setFocus focuses pane p, and blurs the others.
func (m *Modal) setFocus(p pane) {
	m.focus = p
	if p == graphPane {
		m.graph.model.Focus()
	} else {
		m.graph.model.Blur()
	}
	if p == commitPane && m.commit.patch {
		m.commit.pager.Focus()
	} else {
		m.commit.pager.Blur()
	}
	m.layout()
}

// focused returns what the pane that got the focus needs: the commit pane
// reads its commit at once, rather than when the cursor rests.
func (m *Modal) focused() tea.Cmd {
	if m.focus == commitPane {
		return tea.Batch(m.loadDetail(), m.showFile())
	}
	return nil
}

// back steps back from the patch to the files, from the files to the graph
// and from the graph to the branches. It does nothing on the branches.
func (m *Modal) back() tea.Cmd {
	switch {
	case m.focus == commitPane && m.commit.patch:
		m.closePatch()
	case m.focus == commitPane:
		m.setFocus(graphPane)
	case m.focus == graphPane:
		m.setFocus(branchPane)
	}
	return nil
}

// dismiss clears a search or a filter of the patch, and when none is
// left, closes the modal.
func (m *Modal) dismiss() tea.Cmd {
	if m.focus == commitPane && m.commit.patch {
		if cmd, ok := m.commit.pager.ClearTransient(); ok {
			return cmd
		}
	}
	return m.close()
}

// useSelected makes the branch or the commit under the cursor the base.
func (m *Modal) useSelected() tea.Cmd {
	switch m.focus {
	case branchPane:
		br, ok := m.branches.selected()
		if !ok {
			return nil
		}
		return m.useBase(m.branchBase(br.Name))
	case graphPane:
		c, ok := m.selectedCommit()
		if !ok {
			return nil
		}
		return m.useBase(m.commitBase(c.SHA))
	default:
		if !m.commit.has {
			return nil
		}
		return m.useBase(m.commitBase(m.commit.c.SHA))
	}
}

// branchBase is the base at the head of branch name. The default branch's
// head is the base the app starts with.
func (m *Modal) branchBase(name string) ui.BaseMsg {
	if name == m.defaultBranch {
		return ui.BaseMsg{Repo: m.repo}
	}
	return ui.BaseMsg{Repo: m.repo, Ref: name, Label: name, Branch: name}
}

// commitBase is the base at commit sha of the branch shown.
func (m *Modal) commitBase(sha string) ui.BaseMsg {
	branch := m.graph.shown()
	if branch == m.opts.commit {
		// The history of a commit is no branch to open again.
		branch = ""
	}
	return ui.BaseMsg{Repo: m.repo, Ref: sha, Label: label(cmp.Or(branch, m.defaultBranch), sha), Branch: branch}
}

// useBase closes the modal, sets the base, and shows the files at it.
func (m *Modal) useBase(base ui.BaseMsg) tea.Cmd {
	return tea.Sequence(m.close(),
		func() tea.Msg { return base },
		func() tea.Msg { return ui.ShowMsg{Title: ui.FilesTitle} })
}

// open opens what the focused pane shows on GitHub.
func (m *Modal) open() tea.Cmd {
	switch m.focus {
	case branchPane:
		if br, ok := m.branches.selected(); ok {
			return ui.Open(m.branchURL(br.Name))
		}
	case graphPane:
		if c, ok := m.selectedCommit(); ok {
			return ui.Open(m.commitURL(c))
		}
	default:
		c := &m.commit
		switch {
		case !c.has:
		case c.loaded && c.cursor < len(c.files):
			return ui.Open(m.fileURL(c.c, c.files[c.cursor].Path))
		default:
			return ui.Open(m.commitURL(c.c))
		}
	}
	return nil
}

// online reads again, now that GitHub answers again, what failed for want
// of an answer from it: the branches, the history and the commit shown.
// The branches, and the head of the history, served from what an earlier
// read kept while GitHub couldn't be reached or rate limited the read, are
// read again too; the history is shown again if its head moved. What is
// being read already is left to finish.
func (m *Modal) online() tea.Cmd {
	var branches, head tea.Cmd
	switch b := &m.branches; {
	case b.kept && !b.loading:
		// Reading from the first page also reads again a later page
		// that failed.
		b.err = nil
		branches = m.loadBranches("", true)
	case ui.Unreached(b.err):
		branches = m.retryBranches()
	}
	if k := m.graph.kept; k != nil && k.CompareAndSwap(true, false) {
		head = m.readHead()
	}
	return tea.Batch(branches, head, ui.RetryUnreached(&m.graph.model), m.retryCommit(ui.Unreached))
}

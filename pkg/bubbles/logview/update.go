package logview

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Update scrolls, folds and searches on keys while the view is focused,
// and spins while loading whether or not it is.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.state != stateLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		m.flash, m.flashInfo = "", false
		m.pg.ClearNote()
		switch {
		case m.pg.Prompting():
			return m.updateSearch(msg)
		case m.opt:
			return m.updateOption(msg)
		}
		return m.updateKey(msg)
	}
	// The result of a search the pager runs in the background, and the pastes
	// and the like the open prompt takes.
	line, ok := m.pg.Current()
	total := m.pg.Matches()
	searching := m.pg.Searching()
	cmds := []tea.Cmd{m.pg.Forward(msg)}
	if m.focused && m.pg.Prompting() {
		cmds = append(cmds, m.pg.UpdatePrompt(msg))
	}
	// A search that ended goes to the first match at or after the cursor, if
	// the user has not moved it since.
	if searching && !ok && !m.pg.Searching() && m.cursorRow() == m.from {
		m.seekCursor()
		m.syncMatch()
		return m, tea.Batch(cmds...)
	}
	// Follow the pager only to a match it just moved to, so that a fold the
	// user closed over the current match stays closed.
	if l, k := m.pg.Current(); l != line || k != ok || m.pg.Matches() != total {
		m.syncMatch()
	} else {
		m.enableKeys()
	}
	return m, tea.Batch(cmds...)
}

func (m Model) updateKey(k tea.KeyPressMsg) (Model, tea.Cmd) {
	h := max(m.bodyHeight(), 1)
	switch {
	case keymap.Matches(k, m.keys.Down):
		m.move(1)
	case keymap.Matches(k, m.keys.Up):
		m.move(-1)
	case keymap.Matches(k, m.keys.PageDown):
		m.page(h)
	case keymap.Matches(k, m.keys.PageUp):
		m.page(-h)
	case keymap.Matches(k, m.keys.HalfPageDown):
		m.page(max(h/2, 1))
	case keymap.Matches(k, m.keys.HalfPageUp):
		m.page(-max(h/2, 1))
	case keymap.Matches(k, m.keys.Home):
		m.home()
	case keymap.Matches(k, m.keys.End):
		m.end()
	case keymap.Matches(k, m.keys.Right):
		if !m.wrap {
			m.scrollRight(m.hStep())
		}
	case keymap.Matches(k, m.keys.Left):
		m.left = max(m.left-m.hStep(), 0)
	case keymap.Matches(k, m.keys.Expand):
		m.expand()
	case keymap.Matches(k, m.keys.Collapse):
		m.collapse()
	case keymap.Matches(k, m.keys.Toggle):
		m.toggle()
	case keymap.Matches(k, m.keys.ToggleAll):
		m.ToggleAll()
	case keymap.Matches(k, m.keys.NextError):
		m.stepIssue(Error, 1)
	case keymap.Matches(k, m.keys.PrevError):
		m.stepIssue(Error, -1)
	case keymap.Matches(k, m.keys.NextWarning):
		m.stepIssue(Warning, 1)
	case keymap.Matches(k, m.keys.PrevWarning):
		m.stepIssue(Warning, -1)
	case keymap.Matches(k, m.keys.Option):
		m.opt = true
		m.enableKeys()
	case keymap.Matches(k, m.keys.Timestamps):
		m.SetTimeMode(m.times.toggled())
		m.info(m.times.note())
	case keymap.Matches(k, m.keys.Follow):
		m.SetFollow(!m.follow)
	case keymap.Matches(k, m.keys.Search):
		cmd := m.openSearch()
		return m, cmd
	case keymap.Matches(k, m.keys.Next):
		m.step(1)
	case keymap.Matches(k, m.keys.Prev):
		m.step(-1)
	case keymap.Matches(k, m.keys.Quit, m.keys.Dismiss):
		return m, m.close()
	}
	return m, nil
}

func (m Model) updateSearch(k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case keymap.Matches(k, m.keys.Confirm):
		m.syncPosition()
		cmd := m.pg.SubmitPrompt()
		if cmd == nil {
			m.seekCursor()
		}
		m.from = m.cursorRow()
		m.syncMatch()
		return m, cmd
	case keymap.Matches(k, m.keys.Cancel):
		m.closeSearch()
		return m, nil
	}
	cmd := m.pg.UpdatePrompt(k)
	m.enableKeys()
	return m, cmd
}

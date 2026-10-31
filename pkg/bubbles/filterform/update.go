package filterform

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Update handles keys and pastes while focused, the form's own loads and
// spinner ticks, and passes the rest to an open dropdown's picker, which
// takes its own search results and ticks. It ignores messages meant for
// other forms.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.id != m.id {
			return m, nil
		}
		cmd := m.receive(msg)
		return m, cmd
	case spinner.TickMsg:
		if msg.ID != m.spin.ID() {
			cmd := m.passToPicker(msg)
			return m, cmd
		}
		if !m.Loading() {
			m.spinning = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		m.render()
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		m.fields = slices.Clone(m.fields)
		cmd := m.press(msg)
		m.render()
		return m, cmd
	case tea.PasteMsg:
		if !m.focused || m.mode == rowsMode {
			return m, nil
		}
		cmd := m.typeIn(msg)
		m.render()
		return m, cmd
	}
	cmd := m.passToPicker(msg)
	return m, cmd
}

// passToPicker hands msg to the open picker, which takes its own search
// results and ticks and ignores the rest.
func (m *Model) passToPicker(msg tea.Msg) tea.Cmd {
	if !m.picking {
		return nil
	}
	var cmd tea.Cmd
	m.pick, cmd = m.pick.Update(msg)
	m.render()
	return cmd
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	switch m.mode {
	case insertMode:
		return m.pressInsert(msg)
	case listMode:
		return m.pressList(msg)
	case rowsMode:
	}
	k := m.keys
	switch {
	case keymap.Matches(msg, k.Up):
		m.moveTo(m.row - 1)
	case keymap.Matches(msg, k.Down):
		m.moveTo(m.row + 1)
	case keymap.Matches(msg, k.Top):
		m.moveTo(0)
	case keymap.Matches(msg, k.Bottom):
		m.moveTo(m.queryRow() - 1)
	case keymap.Matches(msg, k.NextPane), keymap.Matches(msg, k.PrevPane):
		m.togglePane()
	case keymap.Matches(msg, k.Cancel), keymap.Matches(msg, k.Quit):
		return send(CancelMsg{ID: m.id})
	case keymap.Matches(msg, k.Apply):
		return send(m.applied())
	case m.tabbed() && keymap.Matches(msg, k.NextTab):
		m.switchTab(1)
	case m.tabbed() && keymap.Matches(msg, k.PrevTab):
		m.switchTab(-1)
	case keymap.Matches(msg, k.Prev):
		m.choose(-1)
	case keymap.Matches(msg, k.Next):
		m.choose(1)
	case keymap.Matches(msg, k.Toggle):
		return m.toggle()
	case keymap.Matches(msg, k.Search):
		return m.insert()
	case keymap.Matches(msg, k.Clear):
		m.remove()
	}
	return nil
}

// pressInsert handles a key in insert mode: Leave and Submit act, and the
// rest is typed.
func (m *Model) pressInsert(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys.Typing
	switch {
	case key.Matches(msg, k.Leave):
		m.leaveInsert()
		return nil
	case key.Matches(msg, k.Submit):
		m.leaveInsert()
		return send(m.applied())
	}
	return m.typeIn(msg)
}

// leaveKeepsText is whether leaving insert mode keeps the text, as vim
// does, or puts back the value the field had.
const leaveKeepsText = true

// leaveInsert leaves insert mode, trimming the text of a field.
func (m *Model) leaveInsert() {
	m.closeEditor(leaveKeepsText)
}

// typeIn passes a key or a paste to whatever is being typed in.
func (m *Model) typeIn(msg tea.Msg) tea.Cmd {
	switch {
	case m.mode == listMode && m.picking:
		var cmd tea.Cmd
		m.pick, cmd = m.pick.Update(msg)
		return cmd
	case m.mode != insertMode:
		return nil
	case m.row == m.queryRow():
		before := m.query.Value()
		m.query.SetValue(before)
		var cmd tea.Cmd
		m.query, cmd = m.query.Update(msg)
		if v := m.query.Value(); v != before {
			// Parsing is cheap, so the fields follow every key.
			m.state = parse(&m.spec, v)
		}
		return cmd
	default:
		// The inputs edit their text in place, which copies of the model
		// share, so each gets a copy of its own first.
		m.text.SetValue(m.text.Value())
		var cmd tea.Cmd
		m.text, cmd = m.text.Update(msg)
		m.setValue(m.row, TextValue(m.text.Value()))
		return cmd
	}
}

// moveTo moves the focus to row r, which stops at the first and the last
// row. The query line is a pane of its own, so nothing moves on it.
func (m *Model) moveTo(r int) {
	if m.onQuery() {
		return
	}
	m.row = max(min(r, m.queryRow()-1), 0)
}

// onQuery is whether the query line has the focus, rather than a row.
func (m *Model) onQuery() bool { return m.row == m.queryRow() }

// togglePane moves the focus from the rows to the query line, or back to
// the row it left.
func (m *Model) togglePane() {
	if m.onQuery() {
		m.row = max(min(m.fieldRow, m.queryRow()-1), 0)
		return
	}
	m.fieldRow, m.row = m.row, m.queryRow()
}

// insert starts insert mode on a Text field or the query line, with the
// cursor at the end. Other rows have nothing to type.
func (m *Model) insert() tea.Cmd {
	if !m.types() {
		return nil
	}
	m.mode = insertMode
	in := &m.query
	var cmd tea.Cmd
	if m.row == m.queryRow() {
		cmd = in.Focus()
	} else {
		i := m.row
		in = &m.text
		m.before = m.state.values[i].clone()
		in.Placeholder = m.spec.Fields[i].Hint
		in.SetValue(m.before.text)
		cmd = in.Focus()
	}
	in.CursorEnd()
	return cmd
}

// choose moves the choice of a Choice, of what is sorted by or of the
// order by delta, wrapping around, and flips a Toggle. Other rows have
// nothing to choose.
func (m *Model) choose(delta int) {
	if m.tab == SortTab {
		m.chooseSort(delta)
		return
	}
	if m.row == m.queryRow() {
		return
	}
	f, v := &m.spec.Fields[m.row], m.state.values[m.row]
	switch f.Kind {
	case Choice:
		if len(f.Options) == 0 {
			return
		}
		i := slices.IndexFunc(f.Options, func(it Item) bool { return it.Value == v.text })
		m.setValue(m.row, TextValue(f.Options[cycle(i, delta, len(f.Options))].Value))
	case Toggle:
		m.setValue(m.row, BoolValue(!v.on))
	default:
	}
}

// chooseSort moves the choice of the Sort tab's row in focus by delta.
// An option that writes no sort has no order to choose.
func (m *Model) chooseSort(delta int) {
	sf, so := m.spec.Sort, m.state.sort
	switch m.row {
	case sortByRow:
		if len(sf.Options) == 0 {
			return
		}
		// A new option sorts in its own order, since a direction fit for
		// one, such as the newest first, may not be for the next, such as
		// names. One that writes no sort keeps the order for the next.
		m.sortBy(sf.Options[cycle(sf.index(so.By), delta, len(sf.Options))])
		return
	case sortOrderRow:
		if so.By == "" {
			return
		}
		m.state.sort.Desc = !so.Desc
	}
	m.syncQuery()
}

// sortBy sorts by opt, in its own order, since a direction fit for one,
// such as the newest first, may not be for the next, such as names. An
// option that writes no sort keeps the order for the next.
func (m *Model) sortBy(opt SortOption) {
	desc := m.state.sort.Desc
	if opt.Value != "" {
		desc = !opt.Ascending
	}
	m.state.sort = Sort{By: opt.Value, Desc: desc}
	m.syncQuery()
}

// cycle returns the index delta away from i in n, wrapping around. An i of
// -1, for a value that isn't one of the options, starts from the first.
func cycle(i, delta, n int) int {
	if i < 0 {
		return 0
	}
	return ((i+delta)%n + n) % n
}

// toggle handles space: it flips a Toggle, and opens the dropdown of a
// Choice, what is sorted by, the order, a Multi or a Person.
func (m *Model) toggle() tea.Cmd {
	switch {
	case m.kind() == Toggle:
		m.choose(1)
	case m.opensList():
		return m.openList()
	}
	return nil
}

// canClear reports whether the clear key acts on the row in focus: on a
// field that can be empty and isn't, and on the Sort tab's "sort by" when
// one of its options writes no sort and another is in force. A choice with
// no empty option, an order and the query line have nothing to clear.
func (m *Model) canClear() bool {
	if m.tab == SortTab {
		return m.row == sortByRow && m.spec.Sort != nil && m.spec.Sort.index("") >= 0 && m.state.sort.By != ""
	}
	if m.row >= len(m.spec.Fields) {
		return false
	}
	f := &m.spec.Fields[m.row]
	if f.Kind == Choice && f.Parse == nil && !hasEmpty(f.Options) {
		return false
	}
	return !m.state.values[m.row].IsZero()
}

// remove clears the field in focus, if it can be empty. On the Sort tab it
// chooses the option that writes no sort, which keeps the order for the
// next option.
func (m *Model) remove() {
	if !m.canClear() {
		return
	}
	if m.tab == SortTab {
		m.state.sort = Sort{Desc: m.state.sort.Desc}
		m.syncQuery()
		return
	}
	m.setValue(m.row, Value{})
}

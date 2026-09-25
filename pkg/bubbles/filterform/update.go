package filterform

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys and pastes while focused, the form's own loads and
// spinner ticks, and passes the rest to an open picker. It ignores messages
// meant for other forms.
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
		if !m.focused {
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
	if m.editing {
		return m.pressEditor(msg)
	}
	if m.row == m.queryRow() {
		return m.pressQuery(msg)
	}
	k := m.keys
	switch {
	case key.Matches(msg, k.Up):
		return m.move(-1)
	case key.Matches(msg, k.Down):
		return m.move(1)
	case key.Matches(msg, k.Cancel):
		return send(CancelMsg{ID: m.id})
	case key.Matches(msg, k.Reset):
		m.Reset()
		return nil
	case key.Matches(msg, k.Left):
		m.choose(-1)
		return nil
	case key.Matches(msg, k.Right):
		m.choose(1)
		return nil
	case key.Matches(msg, k.Toggle):
		m.toggle()
		return nil
	case key.Matches(msg, k.Remove):
		m.remove()
		return nil
	case key.Matches(msg, k.Edit) && m.hasEditor():
		return m.openEditor()
	case key.Matches(msg, k.Apply):
		return send(m.applied())
	}
	return nil
}

// pressQuery handles a key on the query line, where every key but the
// moves, apply and cancel is typed.
func (m *Model) pressQuery(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	switch {
	case key.Matches(msg, k.Up):
		return m.move(-1)
	case key.Matches(msg, k.Down):
		return m.move(1)
	case key.Matches(msg, k.Apply):
		return send(m.applied())
	case key.Matches(msg, k.Cancel):
		return send(CancelMsg{ID: m.id})
	}
	return m.typeIn(msg)
}

// typeIn passes a key or a paste to whatever is being typed in.
func (m *Model) typeIn(msg tea.Msg) tea.Cmd {
	switch {
	case m.editing && m.picking:
		var cmd tea.Cmd
		m.pick, cmd = m.pick.Update(msg)
		return cmd
	case m.editing && m.kind() == Text:
		var cmd tea.Cmd
		m.text, cmd = m.text.Update(msg)
		m.setValue(m.row, TextValue(m.text.Value()))
		return cmd
	case m.row == m.queryRow():
		before := m.query.Value()
		var cmd tea.Cmd
		m.query, cmd = m.query.Update(msg)
		if v := m.query.Value(); v != before {
			// Parsing is cheap, so the fields follow every key.
			m.state = parse(&m.spec, v)
			m.resetChips()
		}
		return cmd
	}
	return nil
}

// move moves the focus by delta rows, wrapping around.
func (m *Model) move(delta int) tea.Cmd {
	n := m.queryRow() + 1
	was := m.row
	m.row = ((m.row+delta)%n + n) % n
	if was == m.queryRow() {
		m.query.Blur()
		m.syncQuery()
	}
	if m.row == m.queryRow() {
		cmd := m.query.Focus()
		m.query.CursorEnd()
		return cmd
	}
	return nil
}

// choose moves the choice of a Choice or the sort by delta, flips a Toggle,
// or moves the chip cursor of a Multi.
func (m *Model) choose(delta int) {
	if m.row == m.sortRow() {
		sf := m.spec.Sort
		i := slices.IndexFunc(sf.Options, func(it Item) bool { return it.Value == m.state.sort.By })
		if len(sf.Options) == 0 {
			return
		}
		i = cycle(i, delta, len(sf.Options))
		m.state.sort = Sort{By: sf.Options[i].Value, Desc: m.state.sort.Desc}
		m.syncQuery()
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
	case Multi:
		fs := &m.fields[m.row]
		fs.chip = max(min(fs.chip+delta, len(v.list)), 0)
	default:
	}
}

// cycle returns the index delta away from i in n, wrapping around. An i of
// -1, for a value that isn't one of the options, starts from the first.
func cycle(i, delta, n int) int {
	if i < 0 {
		return 0
	}
	return ((i+delta)%n + n) % n
}

// toggle handles space: it flips the sort's direction and a Toggle, and
// picks the next choice.
func (m *Model) toggle() {
	if m.row == m.sortRow() {
		m.state.sort.Desc = !m.state.sort.Desc
		m.syncQuery()
		return
	}
	if k := m.kind(); k == Choice || k == Toggle {
		m.choose(1)
	}
}

// remove removes the chip under the cursor of a Multi, the last one when
// the cursor is on "+ add", and clears any other field that can be empty.
func (m *Model) remove() {
	if m.row >= len(m.spec.Fields) {
		return
	}
	v := m.state.values[m.row]
	switch m.kind() {
	case Multi:
		if len(v.list) == 0 {
			return
		}
		i := min(m.fields[m.row].chip, len(v.list)-1)
		m.setValue(m.row, Value{list: slices.Delete(slices.Clone(v.list), i, i+1)})
	case Text, Person, Toggle:
		m.setValue(m.row, Value{})
	default:
	}
}

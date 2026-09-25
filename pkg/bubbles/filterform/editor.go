package filterform

import (
	"context"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// hasEditor reports whether the row in focus opens an editor.
func (m *Model) hasEditor() bool {
	k := m.kind()
	return k == Multi || k == Person || k == Text
}

// openEditor opens the editor of the field in focus: a text input for a
// Text, and a picker for a Multi or Person, which first loads its options
// if it has a Loader.
func (m *Model) openEditor() tea.Cmd {
	i := m.row
	m.editing, m.toggled = true, false
	m.before = m.state.values[i].clone()
	if m.kind() == Text {
		m.text.Placeholder = m.spec.Fields[i].Hint
		m.text.SetValue(m.before.text)
		m.text.CursorEnd()
		return m.text.Focus()
	}
	f := &m.spec.Fields[i]
	if f.Load != nil && m.fields[i].state != loaded {
		return m.load(i)
	}
	return m.openPicker(i)
}

// closeEditor closes an open editor, keeping what it changed or putting
// back the value it opened with.
func (m *Model) closeEditor(keep bool) {
	if !m.editing {
		return
	}
	if !keep {
		m.setValue(m.row, m.before)
	}
	m.editing, m.picking, m.toggled = false, false, false
	m.pick = picker.Model{}
	m.text.Blur()
	m.syncQuery()
}

// pressEditor handles a key while an editor is open. Edit keeps what was
// chosen, Cancel puts back what was there, and the rest goes to the
// editor.
func (m *Model) pressEditor(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	i := m.row
	switch {
	case key.Matches(msg, k.Cancel):
		m.closeEditor(false)
		return nil
	case m.kind() == Text:
		if key.Matches(msg, k.Edit) || key.Matches(msg, k.Apply) {
			m.setValue(i, TextValue(strings.TrimSpace(m.text.Value())))
			m.closeEditor(true)
			return nil
		}
		return m.typeIn(msg)
	case !m.picking:
		// The options are loading or failed to.
		if key.Matches(msg, k.Edit) && m.fields[i].state == failed {
			return m.load(i)
		}
		return nil
	case key.Matches(msg, k.Edit):
		m.pickHighlighted(true)
		m.closeEditor(true)
		return nil
	case key.Matches(msg, k.Toggle):
		m.pickHighlighted(false)
		return nil
	}
	return m.typeIn(msg)
}

// pickHighlighted chooses the item the picker highlights. In a Multi,
// space adds or removes it, while enter only adds it, and only if space
// chose nothing, so enter after a few spaces just closes the picker. A
// Person takes the item, or what was typed when nothing matches.
func (m *Model) pickHighlighted(enter bool) {
	i := m.row
	it, ok := m.pick.Selected()
	value, _ := it.Value.(string)
	if m.kind() == Person {
		if !ok {
			value = strings.TrimSpace(m.pick.Query().Text)
		}
		if value != "" {
			m.setValue(i, TextValue(value))
		}
		return
	}
	if !ok || enter && m.toggled {
		return
	}
	list := m.state.values[i].list
	switch j := slices.Index(list, value); {
	case j < 0:
		list = append(slices.Clone(list), value)
	case enter:
		return
	default:
		list = slices.Delete(slices.Clone(list), j, j+1)
	}
	m.toggled = !enter
	m.setValue(i, Value{list: list})
	m.fields[i].chip = len(list)
	m.markPicked(value)
}

// load starts loading field i's options.
func (m *Model) load(i int) tea.Cmd {
	m.seq++
	fs := &m.fields[i]
	fs.state, fs.err, fs.seq = loading, nil, m.seq
	load, ctx, id, seq := m.spec.Fields[i].Load, m.ctx, m.id, m.seq
	cmd := func() tea.Msg {
		items, err := load(ctx, "")
		return loadedMsg{id: id, field: i, seq: seq, items: items, err: err}
	}
	if m.spinning {
		return cmd
	}
	m.spinning = true
	return tea.Batch(cmd, m.spin.Tick)
}

// receive keeps what a load returned, and opens the picker if the field's
// editor is waiting for it.
func (m *Model) receive(msg loadedMsg) tea.Cmd {
	if msg.field >= len(m.fields) {
		return nil
	}
	fs := m.fields[msg.field]
	if fs.state != loading || fs.seq != msg.seq {
		return nil
	}
	m.fields = slices.Clone(m.fields)
	if msg.err != nil {
		fs.state, fs.err = failed, msg.err
	} else {
		fs.state, fs.items = loaded, slices.Clone(msg.items)
	}
	m.fields[msg.field] = fs
	var cmd tea.Cmd
	if m.editing && m.row == msg.field && fs.state == loaded {
		cmd = m.openPicker(msg.field)
	}
	m.render()
	return cmd
}

// openPicker opens the picker of field i over its options. A Person with a
// Loader also searches with it for what the user types.
func (m *Model) openPicker(i int) tea.Cmd {
	f := &m.spec.Fields[i]
	var search picker.Search
	placeholder := "Filter…"
	if f.Kind == Person {
		placeholder = "Filter, or type a login…"
		if load := f.Load; load != nil {
			search = func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
				items, err := load(ctx, q.Text)
				return toPickerItems(items, nil, false), err
			}
		}
	}
	empty := f.Empty
	if empty == "" {
		empty = "Nothing to choose from."
	}
	w, h := m.editorSize()
	m.pick = picker.New(search,
		picker.WithItems(m.pickerItems(i)),
		picker.WithContext(m.ctx),
		picker.WithGroupHeaders(false),
		picker.WithPlaceholder(placeholder),
		picker.WithEmptyText(empty),
		picker.WithKeyMap(m.keys.Picker),
		picker.WithStyles(m.styles.Picker),
		picker.WithSize(w, h),
	)
	m.picking = true
	return tea.Batch(m.pick.Focus(), m.pick.Init())
}

// pickerItems returns field i's options as its picker lists them.
func (m *Model) pickerItems(i int) []picker.Item {
	return toPickerItems(m.items(i), m.state.values[i].list, m.spec.Fields[i].Kind == Multi)
}

// toPickerItems returns items as a picker lists them. With marks, each is
// marked by whether it is in chosen.
func toPickerItems(items []Item, chosen []string, marks bool) []picker.Item {
	out := make([]picker.Item, len(items))
	for j, it := range items {
		title := it.Label
		if title == "" {
			title = it.Value
		}
		if marks {
			if slices.Contains(chosen, it.Value) {
				title = chosenMark + title
			} else {
				title = notChosenMark + title
			}
		}
		out[j] = picker.Item{Title: title, Detail: it.Detail, Value: it.Value}
	}
	return out
}

// markPicked lists the Multi's items again with their marks, and moves the
// highlight back to value, which listing them again moved to the top.
func (m *Model) markPicked(value string) {
	m.pick.SetItems(m.pickerItems(m.row))
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	if !key.Matches(down, m.pick.KeyMap().Down) {
		return
	}
	for range m.pick.Len() {
		if it, ok := m.pick.Selected(); !ok || it.Value == value {
			return
		}
		m.pick, _ = m.pick.Update(down)
	}
}

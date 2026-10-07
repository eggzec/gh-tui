package filterform

import (
	"context"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// openEditor opens the picker of the Multi or Person in focus, which first
// loads its options if it has a Loader.
func (m *Model) openEditor() tea.Cmd {
	i := m.row
	m.mode, m.toggled = pickMode, false
	m.before = m.state.values[i].clone()
	f := &m.spec.Fields[i]
	if f.Load != nil && m.fields[i].state != loaded {
		return m.load(i)
	}
	return m.openPicker(i)
}

// closeEditor leaves insert mode or closes an open picker, keeping what
// was typed or chosen, or putting back the value the field had.
func (m *Model) closeEditor(keep bool) {
	switch m.mode {
	case rowsMode:
		return
	case pickMode:
	case insertMode:
		if m.row == m.queryRow() {
			m.query.Blur()
		} else {
			m.text.Blur()
			if keep {
				m.setValue(m.row, TextValue(strings.TrimSpace(m.text.Value())))
			}
		}
	}
	if !keep && m.row != m.queryRow() {
		m.setValue(m.row, m.before)
	}
	m.mode, m.picking, m.toggled = rowsMode, false, false
	m.pick = picker.Model{}
	m.syncQuery()
}

// pressPick handles a key while a picker is open or waits for its options.
// Apply keeps what was chosen, Cancel puts back what was there, and the
// rest goes to the picker.
func (m *Model) pressPick(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	i := m.row
	switch {
	case key.Matches(msg, k.Cancel):
		m.closeEditor(false)
		return nil
	case !m.picking:
		// The options are loading or failed to.
		if key.Matches(msg, k.Retry) && m.fields[i].state == failed {
			return m.load(i)
		}
		return nil
	case key.Matches(msg, k.Apply):
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
			m.pick.SetMarked([]any{value})
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
	m.markPicked(value)
}

// Err returns why the options of a field failed to load, the first
// field's, or nil if none failed.
func (m Model) Err() error {
	for _, fs := range m.fields {
		if fs.state == failed {
			return fs.err
		}
	}
	return nil
}

// Retry loads again the options of each field whose load failed, as
// opening the field does, such as once the network is back. It returns
// nil if none failed.
func (m *Model) Retry() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.fields {
		if m.fields[i].state != failed {
			continue
		}
		if cmds == nil {
			// Copies of the model share the fields.
			m.fields = slices.Clone(m.fields)
		}
		cmds = append(cmds, m.load(i))
	}
	if cmds == nil {
		return nil
	}
	m.render()
	return tea.Batch(cmds...)
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
	if m.mode == pickMode && m.row == msg.field && fs.state == loaded {
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
	ell := m.styles.Glyphs.Ellipsis
	placeholder := "Filter" + ell
	if f.Kind == Person {
		placeholder = "Filter, or type a login" + ell
		if load := f.Load; load != nil {
			search = func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
				items, err := load(ctx, q.Text)
				return toPickerItems(items, nil, false, m.styles.Glyphs), err
			}
		}
	}
	empty := f.Empty
	if empty == "" {
		empty = "Nothing to choose from."
	}
	w, h := m.editorSize()
	opts := []picker.Option{
		picker.WithItems(m.pickerItems(i)),
		picker.WithContext(m.ctx),
		picker.WithGroupHeaders(false),
		picker.WithPlaceholder(placeholder),
		picker.WithEmptyText(empty),
		picker.WithKeyMap(m.keys.Picker),
		picker.WithStyles(m.styles.Picker),
		picker.WithSize(w, h),
	}
	if say := m.errorText; say != nil {
		opts = append(opts, picker.WithErrorText(func(err error) (text, hint string) {
			text, _ = say(err)
			return text, ""
		}))
	}
	if f.Kind == Person {
		// A Person takes one value, which the list marks, in the options
		// and in what a search finds.
		g := m.styles.Glyphs
		opts = append(opts, picker.WithMarks(g.Chosen, g.NotChosen))
	}
	m.pick = picker.New(search, opts...)
	if f.Kind == Person && m.state.values[i].text != "" {
		m.pick.SetMarked([]any{m.state.values[i].text})
	}
	m.picking = true
	return tea.Batch(m.pick.Focus(), m.pick.Init())
}

// pickerItems returns field i's options as its picker lists them.
func (m *Model) pickerItems(i int) []picker.Item {
	return toPickerItems(m.items(i), m.state.values[i].list, m.spec.Fields[i].Kind == Multi, m.styles.Glyphs)
}

// toPickerItems returns items as a picker lists them. With marks, each is
// marked by whether it is in chosen, with the glyphs g.
func toPickerItems(items []Item, chosen []string, marks bool, g Glyphs) []picker.Item {
	out := make([]picker.Item, len(items))
	for j, it := range items {
		title := it.Label
		if title == "" {
			title = it.Value
		}
		if marks {
			if slices.Contains(chosen, it.Value) {
				title = g.Chosen + " " + title
			} else {
				title = g.NotChosen + " " + title
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

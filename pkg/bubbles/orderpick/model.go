// Package orderpick is a two-pane picker for an ordered list of items. The
// left pane lists what can be chosen, with a filter, and the right pane what
// is chosen, in order, each with a direction that the user can flip.
package orderpick

import (
	"slices"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// Item is something the user can choose.
type Item struct {
	// Value is the parent's own data, which a Choice hands back.
	Value any
	// Title names the item, and Detail follows it in the left pane.
	Title  string
	Detail string
	// AscLabel and DescLabel say what each direction means for the item, such
	// as "A to Z" and "Z to A".
	AscLabel  string
	DescLabel string
	// Desc is the natural direction of the item, which it has when chosen:
	// true for newest first, say, and false for A to Z.
	Desc bool
}

// Pane is one of the two lists.
type Pane int

const (
	// PaneAvailable is the left pane, which lists what can be chosen.
	PaneAvailable Pane = iota
	// PaneChosen is the right pane, which lists what is chosen.
	PaneChosen
)

// narrow is the width below which the panes stack.
const narrow = 60

var lastID atomic.Int64

// chosenItem is an item of the right pane: its place in the items, and its
// direction.
type chosenItem struct {
	index int
	desc  bool
}

// Model is an ordered picker. Create one with [New]. It starts blurred, and
// the parent focuses it when it opens.
type Model struct {
	settings

	id   int64
	left picker.Model
	// chosen is the right pane, in order, and cursor and top its selection
	// and first row shown.
	chosen      []chosenItem
	cursor, top int
	pane        Pane
	focused     bool
	// gutter marks the cursor of the right pane.
	gutter string
}

// New returns an ordered picker with its left pane in focus.
func New(opts ...Option) Model {
	s := defaultSettings()
	for _, opt := range opts {
		opt(&s)
	}
	s.keys = withState(s.keys)
	m := Model{settings: s, id: lastID.Add(1)}
	m.chosen = m.initial()
	m.left = picker.New(nil,
		picker.WithItems(m.available()),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(s.markOn, s.markOff),
		picker.WithKeyMap(s.keys.Picker),
	)
	m.SetStyles(s.styles)
	m.SetSize(s.width, s.height)
	return m
}

// ID returns the instance ID that scopes the picker's messages.
func (m Model) ID() int64 { return m.id }

// Chosen returns the chosen items in their order.
func (m Model) Chosen() []Choice {
	out := make([]Choice, len(m.chosen))
	for i, c := range m.chosen {
		out[i] = Choice{Value: m.items[c.index].Value, Desc: c.desc}
	}
	return out
}

// Pane returns the pane in focus.
func (m Model) Pane() Pane { return m.pane }

// Capturing reports whether the picker takes every key, since the filter of
// the left pane types. A parent asks so it knows not to act on its own
// bindings meanwhile.
func (m Model) Capturing() bool { return m.left.Capturing() }

// Cancel returns the command that sends a CancelMsg, for a parent that closes
// the picker on its back key.
func (m Model) Cancel() tea.Cmd { return send(CancelMsg{ID: m.id}) }

// Focus focuses the picker so it takes keys.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	if m.pane == PaneAvailable {
		return m.left.Focus()
	}
	return nil
}

// Blur blurs the picker so it ignores keys.
func (m *Model) Blur() {
	m.focused = false
	m.left.Blur()
}

// Focused reports whether the picker takes keys.
func (m Model) Focused() bool { return m.focused }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = withState(k)
	m.left.SetKeyMap(m.keys.Picker)
}

// initial returns the right pane the picker starts with, from WithChosen.
func (m Model) initial() []chosenItem {
	var out []chosenItem
	for _, c := range m.start {
		i := slices.IndexFunc(m.items, func(it Item) bool { return it.Value == c.Value })
		if i < 0 || slices.ContainsFunc(out, func(o chosenItem) bool { return o.index == i }) {
			continue
		}
		out = append(out, chosenItem{index: i, desc: c.Desc})
		if m.single {
			break
		}
	}
	return out
}

// available returns the items of the left pane: those not chosen, in their
// own order. A value is the item's place, which is comparable whatever the
// parent's values are.
func (m Model) available() []picker.Item {
	taken := make([]bool, len(m.items))
	for _, c := range m.chosen {
		taken[c.index] = true
	}
	var out []picker.Item
	for i, it := range m.items {
		if !taken[i] {
			out = append(out, picker.Item{Title: it.Title, Detail: it.Detail, Value: i})
		}
	}
	return out
}

// setPane moves the focus to p.
func (m *Model) setPane(p Pane) tea.Cmd {
	m.pane = p
	var cmd tea.Cmd
	if p == PaneAvailable {
		if m.focused {
			cmd = m.left.Focus()
		}
	} else {
		m.left.Blur()
	}
	m.restyle()
	return cmd
}

// tick moves the item under the cursor of the left pane to the end of the
// right pane. The cursor moves to what takes its place, or to what is above it
// when it was the last.
func (m *Model) tick() {
	it, ok := m.left.Selected()
	if !ok {
		return
	}
	next, hasNext := m.left.Following()
	prev, hasPrev := m.left.Preceding()
	index := it.Value.(int)
	if m.single {
		m.chosen = nil
	}
	m.chosen = append(slices.Clone(m.chosen), chosenItem{index: index, desc: m.items[index].Desc})
	m.cursor = len(m.chosen) - 1
	m.scroll()
	m.left.SetItems(m.available())
	switch {
	case hasNext:
		m.left.Select(next.Value)
	case hasPrev:
		m.left.Select(prev.Value)
	}
}

// remove takes the item under the cursor of the right pane back to the left
// pane, where it is among the others as it was, and selected if the filter
// shows it.
func (m *Model) remove() {
	if m.cursor >= len(m.chosen) {
		return
	}
	index := m.chosen[m.cursor].index
	m.chosen = slices.Delete(slices.Clone(m.chosen), m.cursor, m.cursor+1)
	m.moveCursor(m.cursor)
	m.left.SetItems(m.available())
	m.left.Select(index)
}

// shift moves the chosen item under the cursor by d places, and stops at the
// ends.
func (m *Model) shift(d int) {
	to := m.cursor + d
	if to < 0 || to >= len(m.chosen) {
		return
	}
	m.chosen = slices.Clone(m.chosen)
	m.chosen[m.cursor], m.chosen[to] = m.chosen[to], m.chosen[m.cursor]
	m.moveCursor(to)
}

// flip reverses the direction of the chosen item under the cursor.
func (m *Model) flip() {
	if m.cursor >= len(m.chosen) {
		return
	}
	m.chosen = slices.Clone(m.chosen)
	m.chosen[m.cursor].desc = !m.chosen[m.cursor].desc
}

// moveCursor puts the cursor of the right pane on row i, or the nearest one.
func (m *Model) moveCursor(i int) {
	m.cursor = max(min(i, len(m.chosen)-1), 0)
	m.scroll()
}

// scroll keeps the cursor of the right pane in view.
func (m *Model) scroll() {
	rows := m.rows()
	if rows <= 0 {
		m.top = 0
		return
	}
	m.top = max(min(m.top, m.cursor), m.cursor-rows+1, 0)
}

// ShortHelp implements help.KeyMap. It lists the keys of the pane in focus,
// and those of the filter while it types.
func (m Model) ShortHelp() []key.Binding {
	k, n := m.keys, m.keys.Picker.Normal
	apply := help(k.Picker.Choose, "apply")
	switch {
	case m.pane == PaneChosen:
		return []key.Binding{n.Up, n.Down, k.MoveDown, k.MoveUp, k.Flip, k.Remove, apply, k.NextPane, k.Picker.Cancel}
	case m.left.Typing():
		// The keys that switch the scope of the filter are off here.
		return slices.DeleteFunc(m.left.ShortHelp(), func(b key.Binding) bool { return !b.Enabled() })
	}
	return []key.Binding{n.Up, n.Down, n.Search, n.Toggle, apply, k.NextPane, k.Picker.Cancel}
}

// FullHelp implements help.KeyMap. It lists the keys of the pane in focus,
// and those of the filter while it types, so that no two share a key.
func (m Model) FullHelp() [][]key.Binding {
	k, n := m.keys, m.keys.Picker.Normal
	apply := help(k.Picker.Choose, "apply")
	moves := []key.Binding{n.Up, n.Down, n.PageUp, n.PageDown, n.HalfPageUp, n.HalfPageDown, n.Top, n.Bottom}
	switch {
	case m.pane == PaneChosen:
		return [][]key.Binding{
			moves,
			{k.MoveDown, k.MoveUp, k.Flip, k.Remove},
			{apply, k.Picker.Cancel, k.NextPane, k.PrevPane},
		}
	case m.left.Typing():
		return m.left.FullHelp()
	}
	return [][]key.Binding{
		moves,
		{n.Search, n.Toggle},
		{apply, k.Picker.Cancel, k.NextPane, k.PrevPane},
	}
}

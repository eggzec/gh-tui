package filterform

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// KeyMap holds the key bindings of a form. It implements help.KeyMap.
//
// The form works in modes. In rows mode, where it opens, letters are
// keys: they move, change a value, or start typing. Only the keys of
// Typing, and those of an open picker, act while a text input, the query
// line or a picker takes what is typed.
type KeyMap struct {
	// NextTab and PrevTab switch between the Filters and Sort tabs.
	NextTab key.Binding
	PrevTab key.Binding
	// Up and Down move between the rows and the query line, and stop at
	// the ends. Top and Bottom go to the first row and the query line.
	Up     key.Binding
	Down   key.Binding
	Top    key.Binding
	Bottom key.Binding
	// Prev and Next change the value of a Choice, what is sorted by and
	// the order, and flip a Toggle.
	Prev key.Binding
	Next key.Binding
	// Toggle flips a Toggle and opens the picker of a Multi or Person.
	Toggle key.Binding
	// Insert and Append start typing in a Text field or the query line,
	// with the cursor at the start or at the end.
	Insert key.Binding
	Append key.Binding
	// Apply sends an AppliedMsg from any row. In a picker it chooses the
	// highlighted item, or closes a Multi's picker keeping what was
	// chosen.
	Apply key.Binding
	// Clear clears the field in focus. Backspace clears too, since the
	// form has no level to step back to.
	Clear key.Binding
	// Cancel sends a CancelMsg, or closes an open picker and puts back
	// what it changed.
	Cancel key.Binding
	// Quit sends a CancelMsg from the rows.
	Quit key.Binding
	// Retry loads again the options of a field that failed to load.
	Retry key.Binding
	// Typing holds the keys of insert mode.
	Typing TypingKeyMap
	// Picker holds the keys of the picker in a Multi or Person editor. Its
	// Choose and Cancel are taken by Apply and Cancel.
	Picker picker.KeyMap
}

// TypingKeyMap holds the keys of insert mode, which takes every other key
// as text.
type TypingKeyMap struct {
	// Submit sends an AppliedMsg, after it keeps what was typed.
	Submit key.Binding
	// Leave goes back to the rows, keeping what was typed.
	Leave key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		NextTab: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next tab")),
		PrevTab: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous tab")),
		Up:      key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "previous field")),
		Down:    key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next field")),
		Top:     key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "first field")),
		Bottom:  key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "query")),
		Prev:    key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h", "previous")),
		Next:    key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l", "next")),
		Toggle:  key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
		Insert:  key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "insert")),
		Append:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "append")),
		Apply:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "apply")),
		Clear:   key.NewBinding(key.WithKeys("delete", "backspace"), key.WithHelp("delete", "clear")),
		Cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		Quit:    key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "close")),
		Retry:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry")),
		Typing: TypingKeyMap{
			Submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "apply")),
			Leave:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "done")),
		},
		Picker: picker.DefaultKeyMap(),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.Next, k.Apply, k.Cancel}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom, k.Prev, k.Next, k.Toggle},
		{k.Insert, k.Append, k.Clear, k.Apply, k.Cancel, k.Quit, k.Retry},
		{k.NextTab, k.PrevTab},
		{k.Typing.Submit, k.Typing.Leave},
		{
			k.Picker.Up, k.Picker.Down, k.Picker.PageUp, k.Picker.PageDown,
			k.Picker.Choose, k.Picker.Cancel, k.Picker.NextScope, k.Picker.PrevScope,
		},
	}
}

// pair returns a binding that lists a and b as one key, such as "j/k", with
// the help text desc.
func pair(a, b key.Binding, desc string) key.Binding {
	out := a
	out.SetHelp(a.Help().Key+"/"+b.Help().Key, desc)
	return out
}

// relabel returns b with its help text set to desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

// changes reports whether Prev and Next act on the row in focus: they
// change a choice, what is sorted by, the order once there is one, and
// flip a toggle.
func (m *Model) changes() bool {
	if m.row == m.queryRow() {
		return false
	}
	if m.tab == SortTab {
		if m.row == sortByRow {
			return m.spec.Sort != nil && len(m.spec.Sort.Options) > 0
		}
		return m.state.sort.By != ""
	}
	f := &m.spec.Fields[m.row]
	return f.Kind == Choice && len(f.Options) > 0 || f.Kind == Toggle
}

// opensPicker reports whether Toggle opens a picker on the row in focus.
func (m *Model) opensPicker() bool {
	k := m.kind()
	return k == Multi || k == Person
}

// types reports whether Insert and Append start typing on the row in
// focus: a Text field or the query line.
func (m *Model) types() bool {
	return m.row == m.queryRow() || m.kind() == Text
}

// ShortHelp implements help.KeyMap. It lists the keys that act in the mode
// and on the row in focus.
func (m Model) ShortHelp() []key.Binding {
	items := m.helpItems()
	out := make([]key.Binding, len(items))
	for i, it := range items {
		out[i] = it.Binding
	}
	return out
}

// How much a hint matters on the help line. When the line is too wide the
// hints with the highest rank go first, so that the keys that apply and
// close always show.
const (
	rankKeep = iota
	rankMove
	rankChange
	rankClear
	rankTab
)

// hint is a binding on the help line, and its rank.
type hint struct {
	key.Binding
	rank int
}

func (m *Model) helpItems() []hint {
	k := m.keys
	h := func(b key.Binding, rank int) hint { return hint{b, rank} }
	switch m.mode {
	case insertMode:
		return []hint{h(k.Typing.Submit, rankKeep), h(k.Typing.Leave, rankKeep)}
	case pickMode:
		switch {
		case m.picking && m.kind() == Multi:
			return []hint{h(m.moving(), rankMove), h(relabel(k.Toggle, "choose"), rankKeep), h(relabel(k.Apply, "done"), rankKeep), h(relabel(k.Cancel, "back"), rankKeep)}
		case m.picking:
			return []hint{h(m.moving(), rankMove), h(relabel(k.Apply, "choose"), rankKeep), h(relabel(k.Cancel, "back"), rankKeep)}
		case m.fields[m.row].state == failed:
			return []hint{h(k.Retry, rankKeep), h(relabel(k.Cancel, "back"), rankKeep)}
		}
		return []hint{h(relabel(k.Cancel, "back"), rankKeep)}
	case rowsMode:
	}
	out := []hint{h(pair(k.Down, k.Up, "field"), rankMove)}
	if m.changes() {
		out = append(out, h(pair(k.Prev, k.Next, "change"), rankChange))
	}
	switch {
	case m.kind() == Toggle:
		out = append(out, h(k.Toggle, rankChange))
	case m.opensPicker():
		out = append(out, h(relabel(k.Toggle, "list"), rankChange))
	case m.types():
		out = append(out, h(k.Insert, rankChange), h(k.Append, rankChange))
	}
	if m.canClear() {
		out = append(out, h(k.Clear, rankClear))
	}
	out = append(out, h(k.Apply, rankKeep), h(k.Cancel, rankKeep))
	// The key to the other tab comes last, since the tabs show already.
	if m.tabbed() {
		out = append(out, h(m.tabHelp(), rankTab))
	}
	return out
}

// moving is the hint for the keys that move in the open picker, named by
// the first key of its up and down bindings, such as "↑/↓".
func (m *Model) moving() key.Binding {
	km := m.keys.Picker
	if m.picking {
		km = m.pick.KeyMap()
	}
	first := func(b key.Binding) string {
		k, _, _ := strings.Cut(b.Help().Key, "/")
		return k
	}
	return key.NewBinding(key.WithHelp(first(km.Up)+"/"+first(km.Down), "move"))
}

// tabHelp returns the key to the other tab, named after it.
func (m *Model) tabHelp() key.Binding {
	return relabel(m.keys.NextTab, tabHelpDescs[(m.tab+1)%numTabs])
}

// FullHelp implements help.KeyMap. It enables the keys that act in the mode
// and on the row in focus: insert mode takes Typing's, an open picker
// Apply, Toggle, Cancel and its own moves, and the rows the moves, Apply,
// Cancel and Quit, and what the row in focus takes of the rest; other keys
// are typed. Apply and Cancel stand in for the picker's Choose and Cancel.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	if m.picking {
		k.Picker = m.pick.KeyMap()
	}
	form := []*key.Binding{
		&k.NextTab, &k.PrevTab, &k.Up, &k.Down, &k.Top, &k.Bottom, &k.Prev, &k.Next, &k.Toggle,
		&k.Insert, &k.Append, &k.Apply, &k.Clear, &k.Cancel, &k.Quit, &k.Retry,
		&k.Typing.Submit, &k.Typing.Leave,
	}
	pick := []*key.Binding{&k.Picker.Up, &k.Picker.Down, &k.Picker.PageUp, &k.Picker.PageDown, &k.Picker.NextScope, &k.Picker.PrevScope}
	var on []*key.Binding
	switch {
	case m.mode == insertMode:
		on = []*key.Binding{&k.Typing.Submit, &k.Typing.Leave}
	case m.picking:
		on = append([]*key.Binding{&k.Apply, &k.Toggle, &k.Cancel}, pick...)
	case m.mode == pickMode && m.fields[m.row].state == failed:
		on = []*key.Binding{&k.Retry, &k.Cancel}
	case m.mode == pickMode:
		on = []*key.Binding{&k.Cancel}
	default:
		on = []*key.Binding{&k.Up, &k.Down, &k.Top, &k.Bottom, &k.Apply, &k.Cancel, &k.Quit}
		if m.tabbed() {
			on = append(on, &k.NextTab, &k.PrevTab)
		}
		if m.changes() {
			on = append(on, &k.Prev, &k.Next)
		}
		if m.kind() == Toggle || m.opensPicker() {
			on = append(on, &k.Toggle)
			if m.opensPicker() {
				k.Toggle = relabel(k.Toggle, "list")
			}
		}
		if m.types() {
			on = append(on, &k.Insert, &k.Append)
		}
		if m.canClear() {
			on = append(on, &k.Clear)
		}
	}
	for _, b := range slices.Concat(form, pick, []*key.Binding{&k.Picker.Choose, &k.Picker.Cancel}) {
		if !slices.Contains(on, b) {
			b.SetEnabled(false)
		}
	}
	return k.FullHelp()
}

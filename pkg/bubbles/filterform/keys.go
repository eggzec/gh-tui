package filterform

import (
	"slices"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// KeyMap holds the key bindings of a form. It implements help.KeyMap.
//
// The form works in modes. In rows mode, where it opens, letters are
// keys: they move, change a value, or start typing. With a dropdown open
// it works in list mode, where the keys of List and of the two list keys
// act, and letters move; only while the dropdown's filter, a text input or
// the query line takes what is typed do just the keys of Typing act.
type KeyMap struct {
	// NextTab and PrevTab switch between the Filters and Sort tabs.
	NextTab key.Binding `keymap:"global.next_tab" help:"next tab"`
	PrevTab key.Binding `keymap:"global.prev_tab" help:"previous tab"`
	// Up and Down move between the rows and the query line, and stop at
	// the ends. Top and Bottom go to the first row and the query line.
	Up     key.Binding `keymap:"up" help:"previous field"`
	Down   key.Binding `keymap:"down" help:"next field"`
	Top    key.Binding `keymap:"top" help:"first field"`
	Bottom key.Binding `keymap:"bottom" help:"query"`
	// Prev and Next change the value of a Choice, what is sorted by and
	// the order, and flip a Toggle.
	Prev key.Binding `keymap:"left" help:"previous"`
	Next key.Binding `keymap:"right" help:"next"`
	// Toggle flips a Toggle and opens the dropdown of a Choice, of what is
	// sorted by and the order, and of a Multi or Person.
	Toggle key.Binding `keymap:"toggle" help:"toggle"`
	// Insert and Append start typing in a Text field or the query line,
	// with the cursor at the start or at the end.
	Insert key.Binding `keymap:"insert" help:"insert"`
	Append key.Binding `keymap:"append" help:"append"`
	// Apply sends an AppliedMsg from any row. A dropdown has its own
	// enter, List.Choose.
	Apply key.Binding `keymap:"global.select" help:"apply"`
	// Clear clears the field in focus. Backspace clears too, since the
	// form has no level to step back to.
	Clear key.Binding `keymap:"clear" help:"clear"`
	// Cancel sends a CancelMsg. A dropdown has its own esc, List.Cancel.
	Cancel key.Binding `keymap:"global.dismiss" help:"close"`
	// Quit sends a CancelMsg from the rows and from a dropdown, unless its
	// filter takes the keys.
	Quit key.Binding `keymap:"global.quit" help:"close"`
	// Retry loads again the options of a field that failed to load, in
	// its dropdown.
	Retry key.Binding `keymap:"global.refresh" help:"retry"`
	// ListToggle checks or unchecks the highlighted item of a Multi's
	// dropdown, and ListClear chooses the empty option of a list, or
	// unchecks every item of a Multi's.
	ListToggle key.Binding `keymap:"picker.toggle" help:"toggle"`
	ListClear  key.Binding `keymap:"clear" help:"clear"`
	// Typing holds the keys of insert mode.
	Typing TypingKeyMap `keymap:"filter_query"`
	// List holds the keys of a dropdown, a picker with modes: its Normal
	// keys move and open its filter, and Choose and Cancel choose and
	// close. While its filter types, Typing's Up and Down move in place of
	// List's, which are unused, and Cancel leaves the filter.
	List picker.KeyMap `keymap:"picker"`
}

// TypingKeyMap holds the keys of insert mode, which takes every other key
// as text.
type TypingKeyMap struct {
	// Submit sends an AppliedMsg, after it keeps what was typed.
	Submit key.Binding `keymap:"apply" help:"apply"`
	// Leave goes back to the rows, keeping what was typed, or from a
	// dropdown's filter to its list.
	Leave key.Binding `keymap:"cancel" help:"done"`
	// Up and Down move the highlight of a dropdown while its filter takes
	// the letters.
	Up   key.Binding `keymap:"up" help:"up"`
	Down key.Binding `keymap:"down" help:"down"`
}

// NewKeyMap returns the key bindings that look gives for the actions of the
// form, which the tags of its fields name: the form's own are those of
// the context look is for, such as "filter", and the nested maps take
// those of their own contexts. An action without keys gives a disabled
// binding that keeps its help text.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	// The form words its dropdown's choose and close its own way, over what
	// the picker's tags say.
	k.List.Choose.SetHelp(k.List.Choose.Help().Key, "choose")
	k.List.Cancel.SetHelp(k.List.Cancel.Help().Key, "close")
	return k
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
		{k.Typing.Submit, k.Typing.Leave, k.Typing.Up, k.Typing.Down},
		{k.ListToggle, k.ListClear, k.List.Choose, k.List.Cancel},
		{
			k.List.Up, k.List.Down, k.List.PageUp, k.List.PageDown,
			k.List.NextScope, k.List.PrevScope,
		},
		{
			k.List.Normal.Up, k.List.Normal.Down, k.List.Normal.PageUp, k.List.Normal.PageDown,
			k.List.Normal.HalfPageUp, k.List.Normal.HalfPageDown, k.List.Normal.Top, k.List.Normal.Bottom,
			k.List.Normal.Insert, k.List.Normal.Append,
		},
	}
}

// pair returns a binding that lists a and b as one key, such as "j/k", with
// the help text desc.
func pair(a, b key.Binding, desc string) key.Binding {
	out := a
	out.SetHelp(firstKey(a)+"/"+firstKey(b), desc)
	return out
}

// relabel returns b with its help text set to desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(firstKey(b), desc)
	return b
}

// firstKey is the label of the first key of b, which names it on the help
// line, where the room is short, while full help lists every key.
func firstKey(b key.Binding) string {
	if ks := b.Keys(); len(ks) > 0 {
		return keymap.Label(ks[0])
	}
	return b.Help().Key
}

// hinted returns b named by its first key on the help line.
func hinted(b key.Binding) key.Binding { return relabel(b, b.Help().Desc) }

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

// opensList reports whether Toggle opens a dropdown on the row in focus.
func (m *Model) opensList() bool {
	_, ok := m.listOf()
	return ok
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
		return []hint{h(hinted(k.Typing.Submit), rankKeep), h(k.Typing.Leave, rankKeep)}
	case listMode:
		return m.listHints()
	case rowsMode:
	}
	out := []hint{h(pair(k.Down, k.Up, "field"), rankMove)}
	if m.changes() {
		out = append(out, h(pair(k.Prev, k.Next, "change"), rankChange))
	}
	switch {
	case m.kind() == Toggle:
		out = append(out, h(hinted(k.Toggle), rankChange))
	case m.opensList():
		out = append(out, h(relabel(k.Toggle, "list"), rankChange))
	case m.types():
		out = append(out, h(hinted(k.Insert), rankChange), h(hinted(k.Append), rankChange))
	}
	if m.canClear() {
		out = append(out, h(hinted(k.Clear), rankClear))
	}
	out = append(out, h(hinted(k.Apply), rankKeep), h(hinted(k.Cancel), rankKeep))
	// The key to the other tab comes last, since the tabs show already.
	if m.tabbed() {
		out = append(out, h(m.tabHelp(), rankTab))
	}
	return out
}

// listHints returns the hints of list mode: the keys of the dropdown
// open on the row in focus, or of what stands in its place while it loads
// or when that failed.
func (m *Model) listHints() []hint {
	k := m.keys
	h := func(b key.Binding, rank int) hint { return hint{b, rank} }
	closing := relabel(k.List.Cancel, "close")
	multi := m.kind() == Multi
	switch {
	case !m.picking && m.row < len(m.fields) && m.tab == FiltersTab && m.fields[m.row].state == failed:
		return []hint{h(hinted(k.Retry), rankKeep), h(closing, rankKeep)}
	case !m.picking:
		return []hint{h(closing, rankKeep)}
	case m.pick.Typing():
		choose := relabel(k.List.Choose, "choose")
		if multi {
			choose = relabel(k.List.Choose, "toggle")
		}
		return []hint{h(m.moving(), rankMove), h(choose, rankKeep), h(relabel(k.Typing.Leave, "done"), rankKeep)}
	}
	out := []hint{h(m.moving(), rankMove)}
	if multi {
		out = append(out, h(relabel(k.ListToggle, "toggle"), rankChange))
	}
	if m.pick.KeyMap().Normal.Insert.Enabled() {
		out = append(out, h(relabel(k.List.Normal.Insert, "filter"), rankChange))
	}
	if m.canClear() {
		out = append(out, h(hinted(k.ListClear), rankClear))
	}
	choose := relabel(k.List.Choose, "choose")
	if multi {
		choose = relabel(k.List.Choose, m.doneWord())
	}
	return append(out, h(choose, rankKeep), h(closing, rankKeep))
}

// moving is the hint for the keys that move in the open dropdown, named by
// the first key of its up and down bindings, such as "j/k", or "↑/↓" while
// its filter types.
func (m *Model) moving() key.Binding {
	first, second := m.keys.List.Normal.Down, m.keys.List.Normal.Up
	if m.picking && m.pick.Typing() {
		first, second = m.keys.Typing.Up, m.keys.Typing.Down
	}
	return key.NewBinding(key.WithHelp(firstKey(first)+"/"+firstKey(second), "move"))
}

// tabHelp returns the key to the other tab, named after it.
func (m *Model) tabHelp() key.Binding {
	return relabel(m.keys.NextTab, tabHelpDescs[(m.tab+1)%numTabs])
}

// FullHelp implements help.KeyMap. It enables the keys that act in the mode
// and on the row in focus: insert mode takes Typing's Submit and Leave, an
// open dropdown the keys of List, those of its mode, and what the kind of
// list takes of the rest, and the rows the moves, Apply, Cancel and Quit,
// and what the row in focus takes of the rest; other keys are typed. Apply
// and Cancel are the form's enter and esc in the rows, and List's Choose
// and Cancel those of a dropdown, which word them for the kind of list.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	if m.picking {
		up, down := k.List.Up, k.List.Down
		k.List = m.pick.KeyMap()
		k.List.Up, k.List.Down = up, down
	}
	n := &k.List.Normal
	form := []*key.Binding{
		&k.NextTab, &k.PrevTab, &k.Up, &k.Down, &k.Top, &k.Bottom, &k.Prev, &k.Next, &k.Toggle,
		&k.Insert, &k.Append, &k.Apply, &k.Clear, &k.Cancel, &k.Quit, &k.Retry, &k.ListToggle, &k.ListClear,
		&k.Typing.Submit, &k.Typing.Leave, &k.Typing.Up, &k.Typing.Down,
	}
	list := []*key.Binding{
		&k.List.Up, &k.List.Down, &k.List.PageUp, &k.List.PageDown, &k.List.NextScope, &k.List.PrevScope,
		&k.List.Choose, &k.List.Cancel,
		&n.Up, &n.Down, &n.PageUp, &n.PageDown, &n.HalfPageUp, &n.HalfPageDown, &n.Top, &n.Bottom, &n.Insert, &n.Append,
	}
	tabs := func(on []*key.Binding) []*key.Binding {
		if m.tabbed() {
			on = append(on, &k.NextTab, &k.PrevTab)
		}
		return on
	}
	var on []*key.Binding
	switch {
	case m.mode == insertMode:
		on = []*key.Binding{&k.Typing.Submit, &k.Typing.Leave}
	case m.mode == listMode && m.picking && m.pick.Typing():
		on = []*key.Binding{&k.Typing.Up, &k.Typing.Down, &k.List.PageUp, &k.List.PageDown, &k.List.Choose, &k.Typing.Leave}
	case m.mode == listMode && m.picking:
		on = tabs([]*key.Binding{
			&k.List.Choose, &k.List.Cancel, &k.Quit,
			&n.Up, &n.Down, &n.PageUp, &n.PageDown, &n.HalfPageUp, &n.HalfPageDown, &n.Top, &n.Bottom, &n.Insert, &n.Append,
		})
		if m.kind() == Multi {
			on = append(on, &k.ListToggle)
		}
		if m.canClear() {
			on = append(on, &k.ListClear)
		}
	case m.mode == listMode && m.tab == FiltersTab && m.fields[m.row].state == failed:
		on = tabs([]*key.Binding{&k.Retry, &k.List.Cancel, &k.Quit})
	case m.mode == listMode:
		on = tabs([]*key.Binding{&k.List.Cancel, &k.Quit})
	default:
		on = tabs([]*key.Binding{&k.Up, &k.Down, &k.Top, &k.Bottom, &k.Apply, &k.Cancel, &k.Quit})
		if m.changes() {
			on = append(on, &k.Prev, &k.Next)
		}
		if m.kind() == Toggle || m.opensList() {
			on = append(on, &k.Toggle)
			if m.opensList() {
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
	for _, b := range slices.Concat(form, list) {
		if !slices.Contains(on, b) {
			b.SetEnabled(false)
		}
	}
	return k.FullHelp()
}

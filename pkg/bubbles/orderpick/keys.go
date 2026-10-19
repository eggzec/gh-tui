package orderpick

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// KeyMap holds the key bindings of an ordered picker. It implements
// help.KeyMap.
//
// The left pane is a picker with modes, so its keys are those of Picker: in
// its normal mode the keys of Picker.Normal move, search and tick, and while
// its search types, the picker's own take over. The right pane uses the moves
// of Picker.Normal and the keys below. Choose and Cancel act from both panes.
type KeyMap struct {
	// NextPane and PrevPane switch between the panes. They are not taken
	// while the search of the left pane types.
	NextPane key.Binding `keymap:"global.next_pane" help:"next pane"`
	PrevPane key.Binding `keymap:"global.prev_pane" help:"previous pane"`
	// MoveDown and MoveUp move the chosen item under the cursor of the right
	// pane one place down or up the order.
	MoveDown key.Binding `keymap:"move_down" help:"move down"`
	MoveUp   key.Binding `keymap:"move_up" help:"move up"`
	// Flip reverses the direction of the chosen item under the cursor.
	Flip key.Binding `keymap:"flip" help:"reverse"`
	// Remove takes the chosen item under the cursor back to the left pane.
	Remove key.Binding `keymap:"remove" help:"remove"`
	// Picker holds the keys of the left pane. Its Choose sends a ChosenMsg
	// from either pane, and its Cancel a CancelMsg, after it has cleared the
	// query of the left pane. Its scope keys and Normal.Back are unused: the
	// panes have no scopes, and the parent calls Cancel on the back key.
	Picker picker.KeyMap `keymap:"picker"`
}

// NewKeyMap returns the key bindings that look gives for the actions of the
// ordered picker, which the tags of its fields name. An action without keys
// gives a disabled binding that keeps its help text.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	return withState(k)
}

// withState disables the keys the panes have no use for.
func withState(k KeyMap) KeyMap {
	k.Picker.NextScope.SetEnabled(false)
	k.Picker.PrevScope.SetEnabled(false)
	k.Picker.Normal.Back.SetEnabled(false)
	return k
}

// ShortHelp returns the bindings for the short help view of the left pane in
// its normal mode.
func (k KeyMap) ShortHelp() []key.Binding {
	n := k.Picker.Normal
	return []key.Binding{n.Up, n.Down, n.Search, n.Toggle, k.Picker.Choose, k.NextPane, k.Picker.Cancel}
}

// FullHelp returns every binding once. A picker lists those of the pane and
// mode in use, since some keys are shared between them.
func (k KeyMap) FullHelp() [][]key.Binding {
	p, n := k.Picker, k.Picker.Normal
	return [][]key.Binding{
		{k.NextPane, k.PrevPane},
		{k.MoveDown, k.MoveUp, k.Flip, k.Remove},
		{p.Up, p.Down, p.PageUp, p.PageDown, p.Choose, p.Cancel, p.NextScope, p.PrevScope},
		{n.Up, n.Down, n.PageUp, n.PageDown, n.HalfPageUp, n.HalfPageDown, n.Top, n.Bottom, n.Search, n.Toggle, n.Back},
	}
}

// help is a copy of b with desc for its help text.
func help(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

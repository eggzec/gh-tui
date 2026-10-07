package picker

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a picker. Every other key edits the
// query, so none of these are letters, except those of Normal, which a
// picker uses only in normal mode. It implements help.KeyMap, for a picker
// without modes.
type KeyMap struct {
	Up       key.Binding `keymap:"up" help:"up"`
	Down     key.Binding `keymap:"down" help:"down"`
	PageUp   key.Binding `keymap:"page_up" help:"page up"`
	PageDown key.Binding `keymap:"page_down" help:"page down"`
	// Choose sends a ChosenMsg with the selected item.
	Choose key.Binding `keymap:"choose" help:"open"`
	// Cancel sends a CancelMsg.
	Cancel key.Binding `keymap:"cancel" help:"close"`
	// NextScope and PrevScope cycle through the scopes. The picker enables
	// them only when it has scopes.
	NextScope key.Binding `keymap:"next_scope" help:"scope"`
	PrevScope key.Binding `keymap:"prev_scope" help:"previous scope"`
	// Normal holds the keys of normal mode, which a picker built with
	// WithModes starts in. Other pickers ignore them.
	Normal NormalKeyMap `keymap:"picker_normal"`
}

// NormalKeyMap holds the key bindings of normal mode, where the input is
// blurred and letters move instead of typing. Normal mode also takes the
// map's Choose and Cancel.
type NormalKeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"half page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"half page down"`
	Top          key.Binding `keymap:"top" help:"top"`
	Bottom       key.Binding `keymap:"bottom" help:"bottom"`
	// Insert and Append focus the input, with the cursor at the start or at
	// the end of the query. They are disabled when the picker has no
	// filter line.
	Insert key.Binding `keymap:"insert" help:"filter"`
	Append key.Binding `keymap:"append" help:"filter at end"`
}

// NewKeyMap returns the key bindings that look gives for the actions
// of the picker, which the tags of its fields name. An action without keys
// gives a disabled binding that keeps its help text.
// The keys of its normal mode are those of the context picker_normal.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	return k
}

// ShortHelp returns the bindings for the short help view of a picker that
// types. A picker with modes lists its own, by mode.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Choose, k.Cancel, k.NextScope}
}

// FullHelp returns the bindings for the full help view of a picker that
// types. A picker with modes lists its own, by mode.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Choose, k.Cancel, k.NextScope, k.PrevScope},
	}
}

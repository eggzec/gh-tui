package finder

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a finder. Every other key edits the
// query, so none of these are letters. It implements help.KeyMap.
type KeyMap struct {
	Up       key.Binding `keymap:"up" help:"up"`
	Down     key.Binding `keymap:"down" help:"down"`
	PageUp   key.Binding `keymap:"page_up" help:"page up"`
	PageDown key.Binding `keymap:"page_down" help:"page down"`
	// Choose sends a ChosenMsg with the selected item.
	Choose key.Binding `keymap:"choose" help:"open"`
	// Cancel sends a CancelMsg.
	Cancel key.Binding `keymap:"cancel" help:"close"`
}

// NewKeyMap returns the key bindings that look gives for the actions
// of the finder, which the tags of its fields name. An action without keys
// gives a disabled binding that keeps its help text.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	return k
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Choose, k.Cancel}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Choose, k.Cancel},
	}
}

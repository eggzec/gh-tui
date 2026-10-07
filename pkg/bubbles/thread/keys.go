package thread

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a thread.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Top          key.Binding `keymap:"top" help:"top"`
	Bottom       key.Binding `keymap:"bottom" help:"bottom"`
	Retry        key.Binding `keymap:"global.refresh" help:"retry"`
	// Toggle shows or hides the code of the diagram on screen.
	Toggle key.Binding `keymap:"global.select" help:"diagram code"`
}

// NewKeyMap returns the key bindings that look gives, where an action is
// named as the thread's own, such as "page_down", or as a context's, such
// as "global.select". A thread without a key map has no key bound.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	return k
}

// ShortHelp implements help.KeyMap: the moves, as a list offers them, and
// the keys that apply now, so that a parent's own keys still fit beside
// them.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.Retry}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.HalfPageUp, k.HalfPageDown, k.Top, k.Bottom},
		{k.Toggle, k.Retry},
	}
}

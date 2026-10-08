package feed

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a feed. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"first"`
	End          key.Binding `keymap:"bottom" help:"last"`
	// Retry repeats a failed fetch. The feed enables it only while a fetch
	// has failed, so help shows it only when it does something.
	Retry key.Binding `keymap:"global.refresh" help:"retry"`
}

// NewKeyMap returns the key bindings that look gives the keys of, such as
// those of the context of the pane that shows the feed. An action without
// keys gives a disabled binding.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	// The feed enables the retry key while a fetch has failed.
	k.Retry.SetEnabled(false)
	return k
}

// unbound is the keys of a feed made without a key map: none.
func unbound(string) []string { return nil }

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Retry}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.Home, k.End, k.Retry},
	}
}

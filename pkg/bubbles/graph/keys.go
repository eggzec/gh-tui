package graph

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a graph. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"newest"`
	// End moves to the last commit loaded. Unless that is the end of the
	// history, the next chunk is fetched, and pressing End again goes on.
	End key.Binding `keymap:"bottom" help:"oldest loaded"`
	// Choose sends a [ChosenMsg] for the commit under the cursor.
	Choose key.Binding `keymap:"global.select" help:"open"`
	// Retry repeats a failed fetch. The graph enables it only while a fetch
	// has failed, so help shows it only when it does something.
	Retry key.Binding `keymap:"global.refresh" help:"retry"`
}

// NewKeyMap returns the key bindings that look gives the keys of, such as
// those of the context of the pane that shows the graph. An action without
// keys gives a disabled binding.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	// The graph enables the retry key while a fetch has failed.
	k.Retry.SetEnabled(false)
	return k
}

// unbound is the keys of a graph made without a key map: none.
var unbound = keymap.Func(func(string) []string { return nil })

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Choose, k.Retry}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.Home, k.End, k.Choose, k.Retry},
	}
}

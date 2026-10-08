package calendar

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a calendar. It implements help.KeyMap.
type KeyMap struct {
	// Up moves to the day before, and from a Sunday to the Saturday of the
	// week before.
	Up key.Binding `keymap:"up" help:"day before"`
	// Down moves to the day after, and from a Saturday to the Sunday of the
	// week after.
	Down key.Binding `keymap:"down" help:"day after"`
	// Left moves to the same weekday of the week before.
	Left key.Binding `keymap:"left" help:"week before"`
	// Right moves to the same weekday of the week after.
	Right key.Binding `keymap:"right" help:"week after"`
	// First moves to the first day.
	First key.Binding `keymap:"top" help:"first day"`
	// Last moves to the last day.
	Last key.Binding `keymap:"bottom" help:"last day"`
}

// NewKeyMap returns the key bindings that look gives the keys of, such as
// those of the context of the pane that shows the calendar. An action without
// keys gives a disabled binding.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	return k
}

// unbound is the keys of a calendar made without a key map: none.
func unbound(string) []string { return nil }

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Up, k.Down}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down},
		{k.First, k.Last},
	}
}

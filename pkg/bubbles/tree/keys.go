package tree

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a tree. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"first"`
	End          key.Binding `keymap:"bottom" help:"last"`
	// Expand expands the branch under the cursor, loading its children if
	// needed, or moves to its first child when it is expanded already. On
	// a leaf it sends an [OpenMsg], as Open does. It also retries a failed
	// load.
	Expand key.Binding `keymap:"expand" help:"expand"`
	// Collapse collapses the branch under the cursor, or moves to the parent
	// on a leaf or a collapsed branch.
	Collapse key.Binding `keymap:"collapse" help:"collapse"`
	// ToggleAll expands every branch, within the limits of
	// [WithExpandAllLimits], or collapses every branch when all are
	// expanded.
	ToggleAll key.Binding `keymap:"toggle_all" help:"all"`
	// Open sends an [OpenMsg] for the leaf under the cursor, or toggles the
	// branch.
	Open key.Binding `keymap:"global.select" help:"open"`
}

// NewKeyMap returns the key bindings that look gives the keys of, such as
// those of the context of the pane that shows the tree. An action without
// keys gives a disabled binding.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	return k
}

// unbound is the keys of a tree made without a key map: none.
func unbound(string) []string { return nil }

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Expand, k.Collapse, k.Open}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End},
		{k.Expand, k.Collapse, k.ToggleAll, k.Open},
	}
}

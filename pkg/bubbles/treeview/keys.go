package treeview

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
	// Expand opens the closed fold under the cursor, or moves to its first
	// child when it is open. On a leaf it does nothing.
	Expand key.Binding `keymap:"expand" help:"expand"`
	// Collapse closes the open fold under the cursor, or moves to the
	// nearest ancestor on a closed fold or a leaf.
	Collapse key.Binding `keymap:"collapse" help:"collapse"`
	// ToggleAll opens every fold if a fold that shows is closed, else
	// closes every fold.
	ToggleAll key.Binding `keymap:"toggle_all" help:"all"`
	// Open sends an [OpenMsg] for the row under the cursor. It never folds.
	Open key.Binding `keymap:"global.select" help:"open"`
	// Mark sends a [MarkMsg] for the row under the cursor and moves to the
	// next row of its kind.
	Mark key.Binding `keymap:"mark" help:"mark"`
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
var unbound = keymap.Func(func(string) []string { return nil })

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Expand, k.Collapse, k.Open}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End},
		{k.Expand, k.Collapse, k.ToggleAll, k.Open, k.Mark},
	}
}

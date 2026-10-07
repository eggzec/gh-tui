package tree

import "charm.land/bubbles/v2/key"

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
	// needed. It also retries a failed load.
	Expand key.Binding `keymap:"expand" help:"expand"`
	// Right expands the branch under the cursor, or moves to its first child
	// when it is expanded already.
	Right key.Binding `keymap:"step_in" help:"expand/enter"`
	// Collapse collapses the branch under the cursor, or moves to the parent
	// on a leaf or a collapsed branch.
	Collapse key.Binding `keymap:"collapse" help:"collapse"`
	// ExpandAll expands the branch under the cursor and every branch below
	// it, within the limits of [WithExpandAllLimits].
	ExpandAll key.Binding `keymap:"expand_all" help:"expand all"`
	// CollapseAll collapses every branch.
	CollapseAll key.Binding `keymap:"collapse_all" help:"collapse all"`
	// Open sends an [OpenMsg] for the leaf under the cursor, or toggles the
	// branch.
	Open key.Binding `keymap:"global.select" help:"open"`
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:       key.NewBinding(key.WithKeys("ctrl+b", "pgup"), key.WithHelp("^b/pgup", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("ctrl+f", "pgdown"), key.WithHelp("^f/pgdn", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ page up")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ page down")),
		Home:         key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/home", "first")),
		End:          key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G/end", "last")),
		Expand:       key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "expand")),
		Right:        key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand/enter")),
		Collapse:     key.NewBinding(key.WithKeys("-", "left", "h"), key.WithHelp("←/h/-", "collapse")),
		ExpandAll:    key.NewBinding(key.WithKeys("*"), key.WithHelp("*", "expand all")),
		CollapseAll:  key.NewBinding(key.WithKeys("="), key.WithHelp("=", "collapse all")),
		Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Right, k.Collapse, k.Open}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End},
		{k.Expand, k.Right, k.Collapse, k.ExpandAll, k.CollapseAll, k.Open},
	}
}

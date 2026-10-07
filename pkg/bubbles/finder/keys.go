package finder

import "charm.land/bubbles/v2/key"

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

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:       key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓", "down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		Choose:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open")),
		Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	}
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

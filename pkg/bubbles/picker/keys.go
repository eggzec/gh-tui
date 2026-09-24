package picker

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a picker. Every other key edits the
// query, so none of these are letters. It implements help.KeyMap.
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	// Choose sends a ChosenMsg with the selected item.
	Choose key.Binding
	// Cancel sends a CancelMsg.
	Cancel key.Binding
	// NextScope and PrevScope cycle through the scopes. The picker enables
	// them only when it has scopes.
	NextScope key.Binding
	PrevScope key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:        key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/ctrl+p", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓/ctrl+n", "down")),
		PageUp:    key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:  key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		Choose:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		NextScope: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "scope")),
		PrevScope: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous scope")),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Choose, k.Cancel, k.NextScope}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Choose, k.Cancel, k.NextScope, k.PrevScope},
	}
}

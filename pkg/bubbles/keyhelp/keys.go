package keyhelp

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of the help. Every other key edits the
// query, so none of these are letters. It implements help.KeyMap.
type KeyMap struct {
	Up       key.Binding `keymap:"up" help:"scroll up"`
	Down     key.Binding `keymap:"down" help:"scroll down"`
	PageUp   key.Binding `keymap:"page_up" help:"page up"`
	PageDown key.Binding `keymap:"page_down" help:"page down"`
	Home     key.Binding `keymap:"top" help:"top"`
	End      key.Binding `keymap:"bottom" help:"bottom"`
	// Capture turns key capture on and off. While it is on, the next key
	// pressed lists the bindings that hold it, and so does each key after
	// it, until Capture again.
	Capture key.Binding `keymap:"capture" help:"find a key"`
	// Back clears the query and the captured key, or sends a [CloseMsg]
	// when there are none.
	Back key.Binding `keymap:"cancel" help:"clear, then close"`
	// Close sends a [CloseMsg] while the query is empty. Otherwise it is
	// typed.
	Close key.Binding `keymap:"close" help:"close"`
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:       key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "scroll up")),
		Down:     key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "scroll down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		Home:     key.NewBinding(key.WithKeys("home"), key.WithHelp("home", "top")),
		End:      key.NewBinding(key.WithKeys("end"), key.WithHelp("end", "bottom")),
		Capture:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "find a key")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear, then close")),
		Close:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "close")),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Capture, k.Back, k.Close}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End},
		{k.Capture, k.Back, k.Close},
	}
}

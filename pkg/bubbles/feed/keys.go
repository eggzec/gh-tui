package feed

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a feed. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Home         key.Binding
	End          key.Binding
	// Retry repeats a failed fetch. The feed enables it only while a fetch
	// has failed, so help shows it only when it does something.
	Retry key.Binding
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
		Retry:        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry"), key.WithDisabled()),
	}
}

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

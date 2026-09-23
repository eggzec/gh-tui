package tabs

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of the tab bar. It implements help.KeyMap.
type KeyMap struct {
	Next key.Binding
	Prev key.Binding
	// Jump matches the digits 1 to 9, which select the tab with that number.
	Jump key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
		Jump: key.NewBinding(
			key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
			key.WithHelp("1-9", "go to tab"),
		),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Prev}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Next, k.Prev, k.Jump}}
}

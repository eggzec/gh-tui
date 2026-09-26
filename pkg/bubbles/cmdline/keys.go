package cmdline

import "charm.land/bubbles/v2/key"

// KeyMap holds the command line's own keys. Editing keys belong to the
// text input underneath and keep their defaults.
type KeyMap struct {
	// Submit sends the line.
	Submit key.Binding
	// Cancel closes the command line without sending the line.
	Cancel key.Binding
	// CancelEmpty closes the command line when the line is empty, as
	// backspace does in vim. On a line with text it edits as usual.
	CancelEmpty key.Binding
	// Next inserts the next candidate, and Prev the one before.
	Next, Prev key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "run"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc", "cancel"),
		),
		CancelEmpty: key.NewBinding(
			key.WithKeys("backspace", "ctrl+h"),
			key.WithHelp("backspace", "cancel when empty"),
		),
		Next: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "complete"),
		),
		Prev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "previous"),
		),
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Next, k.Cancel}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Submit, k.Cancel, k.CancelEmpty}, {k.Next, k.Prev}}
}

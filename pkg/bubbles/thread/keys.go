package thread

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a thread.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Top          key.Binding `keymap:"top" help:"top"`
	Bottom       key.Binding `keymap:"bottom" help:"bottom"`
	Retry        key.Binding `keymap:"global.refresh" help:"retry"`
	// Toggle shows or hides the code of the diagram on screen.
	Toggle key.Binding `keymap:"global.select" help:"diagram code"`
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("b", "ctrl+b", "pgup"),
			key.WithHelp("b/^b", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("space", "ctrl+f", "pgdown"),
			key.WithHelp("space/^f", "page down"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("^u", "½ page up"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("^d", "½ page down"),
		),
		Top: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("g/home", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("G/end", "bottom"),
		),
		Retry: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "retry"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("↵", "diagram code"),
		),
	}
}

// ShortHelp implements help.KeyMap: the moves, as a list offers them, and
// the keys that apply now, so that a parent's own keys still fit beside
// them.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.Retry}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.HalfPageUp, k.HalfPageDown, k.Top, k.Bottom},
		{k.Toggle, k.Retry},
	}
}

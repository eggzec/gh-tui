package graph

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a graph. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"newest"`
	// End moves to the last commit loaded. Unless that is the end of the
	// history, the next chunk is fetched, and pressing End again goes on.
	End key.Binding `keymap:"bottom" help:"oldest loaded"`
	// Choose sends a [ChosenMsg] for the commit under the cursor.
	Choose key.Binding `keymap:"global.select" help:"open"`
	// Retry repeats a failed fetch. The graph enables it only while a fetch
	// has failed, so help shows it only when it does something.
	Retry key.Binding `keymap:"global.refresh" help:"retry"`
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
		Home:         key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/home", "newest")),
		End:          key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G/end", "oldest loaded")),
		Choose:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Retry:        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry"), key.WithDisabled()),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Choose, k.Retry}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.Home, k.End, k.Choose, k.Retry},
	}
}

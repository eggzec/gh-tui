package picker

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a picker. Every other key edits the
// query, so none of these are letters, except those of Normal, which a
// picker uses only in normal mode. It implements help.KeyMap, for a picker
// without modes.
type KeyMap struct {
	Up       key.Binding `keymap:"up" help:"up"`
	Down     key.Binding `keymap:"down" help:"down"`
	PageUp   key.Binding `keymap:"page_up" help:"page up"`
	PageDown key.Binding `keymap:"page_down" help:"page down"`
	// Choose sends a ChosenMsg with the selected item.
	Choose key.Binding `keymap:"choose" help:"open"`
	// Cancel sends a CancelMsg.
	Cancel key.Binding `keymap:"cancel" help:"close"`
	// NextScope and PrevScope cycle through the scopes. The picker enables
	// them only when it has scopes.
	NextScope key.Binding `keymap:"next_scope" help:"scope"`
	PrevScope key.Binding `keymap:"prev_scope" help:"previous scope"`
	// Normal holds the keys of normal mode, which a picker built with
	// WithModes starts in. Other pickers ignore them.
	Normal NormalKeyMap `keymap:"picker_normal"`
}

// NormalKeyMap holds the key bindings of normal mode, where the input is
// blurred and letters move instead of typing. Normal mode also takes the
// map's Choose and Cancel.
type NormalKeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"half page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"half page down"`
	Top          key.Binding `keymap:"top" help:"top"`
	Bottom       key.Binding `keymap:"bottom" help:"bottom"`
	// Insert and Append focus the input, with the cursor at the start or at
	// the end of the query. They are disabled when the picker has no
	// filter line.
	Insert key.Binding `keymap:"insert" help:"filter"`
	Append key.Binding `keymap:"append" help:"filter at end"`
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
		Normal: NormalKeyMap{
			Up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
			Down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
			PageUp:       key.NewBinding(key.WithKeys("ctrl+b", "pgup"), key.WithHelp("^b/pgup", "page up")),
			PageDown:     key.NewBinding(key.WithKeys("ctrl+f", "pgdown"), key.WithHelp("^f/pgdn", "page down")),
			HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "half page up")),
			HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "half page down")),
			Top:          key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g/home", "top")),
			Bottom:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G/end", "bottom")),
			Insert:       key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "filter")),
			Append:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "filter at end")),
		},
	}
}

// ShortHelp returns the bindings for the short help view of a picker that
// types. A picker with modes lists its own, by mode.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Choose, k.Cancel, k.NextScope}
}

// FullHelp returns the bindings for the full help view of a picker that
// types. A picker with modes lists its own, by mode.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Choose, k.Cancel, k.NextScope, k.PrevScope},
	}
}

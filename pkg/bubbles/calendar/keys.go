package calendar

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a calendar. It implements help.KeyMap.
type KeyMap struct {
	// Up moves to the day before, and from a Sunday to the Saturday of the
	// week before.
	Up key.Binding `keymap:"up" help:"day before"`
	// Down moves to the day after, and from a Saturday to the Sunday of the
	// week after.
	Down key.Binding `keymap:"down" help:"day after"`
	// Left moves to the same weekday of the week before.
	Left key.Binding `keymap:"left" help:"week before"`
	// Right moves to the same weekday of the week after.
	Right key.Binding `keymap:"right" help:"week after"`
	// First moves to the first day.
	First key.Binding `keymap:"top" help:"first day"`
	// Last moves to the last day.
	Last key.Binding `keymap:"bottom" help:"last day"`
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "day before")),
		Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "day after")),
		Left:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "week before")),
		Right: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "week after")),
		First: key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/home", "first day")),
		Last:  key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G/end", "last day")),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Up, k.Down}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down},
		{k.First, k.Last},
	}
}

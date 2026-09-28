package pager

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a pager. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Home         key.Binding
	End          key.Binding
	// Left and Right scroll sideways while lines are not wrapped.
	Left  key.Binding
	Right key.Binding

	Wrap        key.Binding
	LineNumbers key.Binding

	// Search opens the search prompt, Confirm searches for the pattern
	// typed, a regexp, or for the lines it doesn't match after a "!", and
	// Cancel closes the prompt. Outside the prompt, Cancel clears the
	// search. The pager enables Confirm only while the prompt is open, and
	// Cancel only while it is or a search or filter is shown, so esc
	// closes the pager otherwise.
	Search  key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	// Filter opens the filter prompt, where Confirm shows only the lines
	// the pattern typed matches, or doesn't match after a "!", and an
	// empty line shows them all again. Outside the prompt, Cancel stops
	// a filter still running, or clears the one shown, once no search is
	// shown.
	Filter key.Binding
	// Next and Prev move between matches. The pager enables them only
	// while there are matches, so help shows them only when they work.
	Next key.Binding
	Prev key.Binding

	// Close asks the parent to close the pager with a [CloseMsg]. While a
	// search is shown, a key bound to Cancel clears it first.
	Close key.Binding
}

// DefaultKeyMap returns the default key bindings, which follow less.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:       key.NewBinding(key.WithKeys("pgup", "b"), key.WithHelp("b/pgup", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown", "space", "f"), key.WithHelp("f/pgdn", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "½ page up")),
		HalfPageDown: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "½ page down")),
		Home:         key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/home", "top")),
		End:          key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G/end", "bottom")),
		Left:         key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:        key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
		Wrap:         key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "wrap")),
		LineNumbers:  key.NewBinding(key.WithKeys("#"), key.WithHelp("#", "line numbers")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Filter:       key.NewBinding(key.WithKeys("&"), key.WithHelp("&", "filter")),
		Confirm:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search"), key.WithDisabled()),
		Cancel:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"), key.WithDisabled()),
		Next:         key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match"), key.WithDisabled()),
		Prev:         key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match"), key.WithDisabled()),
		Close:        key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "close")),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Next, k.Prev, k.Close}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.Home, k.End, k.Left, k.Right, k.Wrap, k.LineNumbers},
		{k.Search, k.Filter, k.Confirm, k.Cancel, k.Next, k.Prev, k.Close},
	}
}

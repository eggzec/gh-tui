package logview

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a log view. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"top"`
	End          key.Binding `keymap:"bottom" help:"bottom"`
	// Left and Right scroll sideways while lines are not wrapped.
	Left  key.Binding `keymap:"left" help:"left"`
	Right key.Binding `keymap:"right" help:"right"`

	// Toggle expands or collapses the section or group under the cursor,
	// or collapses the one the cursor is in.
	Toggle key.Binding `keymap:"global.select" help:"fold"`
	// Expand expands the section or group under the cursor.
	Expand key.Binding `keymap:"expand" help:"expand"`
	// Collapse collapses the section or group under the cursor, or the one
	// the cursor is in.
	Collapse key.Binding `keymap:"collapse" help:"collapse"`
	// FoldAll folds every section when any is open, and expands every
	// section otherwise. Groups keep their state.
	FoldAll key.Binding `keymap:"expand_all" help:"fold all"`

	// NextError and PrevError move to the next and previous error line,
	// expanding what hides it, and NextWarning and PrevWarning to the
	// warnings. The view enables them only while there are some.
	NextError   key.Binding `keymap:"next_error" help:"next error"`
	PrevError   key.Binding `keymap:"prev_error" help:"prev error"`
	NextWarning key.Binding `keymap:"next_warning" help:"next warning"`
	PrevWarning key.Binding `keymap:"prev_warning" help:"prev warning"`

	// Wrap toggles soft-wrapping. It is s by default, since w moves to
	// warnings.
	Wrap key.Binding `keymap:"wrap" help:"wrap"`
	// Times shows the times relative to their section, then the times of
	// day, then hides them.
	Times       key.Binding `keymap:"times" help:"times"`
	LineNumbers key.Binding `keymap:"line_numbers" help:"line numbers"`
	// Follow toggles following appended lines, and moves to the end when
	// it turns on.
	Follow key.Binding `keymap:"follow" help:"follow"`

	// Search opens the search input, Confirm searches for what it holds,
	// and Cancel closes it. Outside the input, Cancel clears the search.
	// The view enables Confirm only while the input is open, and Cancel
	// only while it is or a search is shown, so enter folds and esc closes
	// the view otherwise.
	Search  key.Binding `keymap:"find" help:"search"`
	Confirm key.Binding `keymap:"search_prompt.run" help:"search"`
	Cancel  key.Binding `keymap:"search_prompt.cancel" help:"cancel"`
	// Next and Prev move between matches. The view enables them only while
	// there are matches.
	Next key.Binding `keymap:"next_match" help:"next match"`
	Prev key.Binding `keymap:"prev_match" help:"prev match"`

	// Close asks the parent to close the view with a [CloseMsg]. While a
	// search is shown, a key bound to Cancel clears it first.
	Close key.Binding `keymap:"global.quit" help:"close"`
}

// DefaultKeyMap returns the default key bindings, which follow less for
// scrolling and the tree for folding.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:       key.NewBinding(key.WithKeys("b", "ctrl+b", "pgup"), key.WithHelp("b/^b", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("space", "ctrl+f", "pgdown"), key.WithHelp("space/^f", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ page up")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ page down")),
		Home:         key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/home", "top")),
		End:          key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G/end", "bottom")),
		Left:         key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:        key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
		Toggle:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "fold")),
		Expand:       key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "expand")),
		Collapse:     key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "collapse")),
		FoldAll:      key.NewBinding(key.WithKeys("*"), key.WithHelp("*", "fold all")),
		NextError:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "next error"), key.WithDisabled()),
		PrevError:    key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "prev error"), key.WithDisabled()),
		NextWarning:  key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "next warning"), key.WithDisabled()),
		PrevWarning:  key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "prev warning"), key.WithDisabled()),
		Wrap:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "wrap")),
		Times:        key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "times")),
		LineNumbers:  key.NewBinding(key.WithKeys("#"), key.WithHelp("#", "line numbers")),
		Follow:       key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "follow")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Confirm:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search"), key.WithDisabled()),
		Cancel:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"), key.WithDisabled()),
		Next:         key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match"), key.WithDisabled()),
		Prev:         key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match"), key.WithDisabled()),
		Close:        key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "close")),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Toggle, k.FoldAll, k.NextError, k.Search, k.Next, k.Close}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End},
		{k.Toggle, k.Expand, k.Collapse, k.FoldAll},
		{k.NextError, k.PrevError, k.NextWarning, k.PrevWarning, k.Search, k.Confirm, k.Cancel, k.Next, k.Prev},
		{k.Left, k.Right, k.Wrap, k.Times, k.LineNumbers, k.Follow, k.Close},
	}
}

package logview

import "charm.land/bubbles/v2/key"

// KeyMap holds the key bindings of a log view. It implements help.KeyMap.
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

	// Toggle expands or collapses the section or group under the cursor,
	// or collapses the one the cursor is in.
	Toggle key.Binding
	// Expand expands the section or group under the cursor.
	Expand key.Binding
	// Collapse collapses the section or group under the cursor, or the one
	// the cursor is in.
	Collapse key.Binding
	// FoldAll folds every section when any is open, and expands every
	// section otherwise. Groups keep their state.
	FoldAll key.Binding

	// NextError and PrevError move to the next and previous error line,
	// expanding what hides it, and NextWarning and PrevWarning to the
	// warnings. The view enables them only while there are some.
	NextError   key.Binding
	PrevError   key.Binding
	NextWarning key.Binding
	PrevWarning key.Binding

	// Wrap toggles soft-wrapping. It is s by default, since w moves to
	// warnings.
	Wrap key.Binding
	// Times shows the times relative to their section, then the times of
	// day, then hides them.
	Times       key.Binding
	LineNumbers key.Binding
	// Follow toggles following appended lines, and moves to the end when
	// it turns on.
	Follow key.Binding

	// Search opens the search input, Confirm searches for what it holds,
	// and Cancel closes it. Outside the input, Cancel clears the search.
	// The view enables Confirm only while the input is open, and Cancel
	// only while it is or a search is shown, so enter folds and esc closes
	// the view otherwise.
	Search  key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	// Next and Prev move between matches. The view enables them only while
	// there are matches.
	Next key.Binding
	Prev key.Binding

	// Close asks the parent to close the view with a [CloseMsg]. While a
	// search is shown, a key bound to Cancel clears it first.
	Close key.Binding
}

// DefaultKeyMap returns the default key bindings, which follow less for
// scrolling and the tree for folding.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:       key.NewBinding(key.WithKeys("b", "ctrl+b", "pgup"), key.WithHelp("b/ctrl+b/pgup", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("space", "ctrl+f", "pgdown"), key.WithHelp("space/ctrl+f/pgdn", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "½ page up")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "½ page down")),
		Home:         key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/home", "top")),
		End:          key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G/end", "bottom")),
		Left:         key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:        key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
		Toggle:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "fold")),
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

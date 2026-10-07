package logview

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

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

	// Quit and Dismiss both ask the parent to close the view with a
	// [CloseMsg]: quit as the app's quit key, and dismiss as its key for
	// stepping back out of what is open. While a search is shown, a key
	// bound to Cancel clears it first.
	Quit    key.Binding `keymap:"global.quit" help:"close"`
	Dismiss key.Binding `keymap:"global.dismiss" help:"close"`
}

// NewKeyMap returns the key bindings that look gives, where an action is
// named as the view's own, such as "page_down", or as a context's, such as
// "global.select". A view without a key map has no key bound.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	for _, b := range []*key.Binding{&k.NextError, &k.PrevError, &k.NextWarning, &k.PrevWarning, &k.Confirm, &k.Cancel, &k.Next, &k.Prev} {
		b.SetEnabled(false)
	}
	return k
}

// Close returns the binding that stands for Quit and Dismiss in help, which
// lists them as one row.
func (k KeyMap) Close() key.Binding { return keymap.Join(k.Quit, k.Dismiss) }

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Toggle, k.FoldAll, k.NextError, k.Search, k.Next, k.Close()}
}

// FullHelp returns the bindings for the full help view, every binding of
// the key map once. The model's own full help, which is what help reads,
// lists Quit and Dismiss as one row, [KeyMap.Close].
func (k KeyMap) FullHelp() [][]key.Binding { return k.fullHelp(k.Quit, k.Dismiss) }

// fullHelp returns the full help with closing as the bindings that close
// the view, listed together.
func (k KeyMap) fullHelp(closing ...key.Binding) [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End},
		{k.Toggle, k.Expand, k.Collapse, k.FoldAll},
		{k.NextError, k.PrevError, k.NextWarning, k.PrevWarning, k.Search, k.Confirm, k.Cancel, k.Next, k.Prev},
		append([]key.Binding{k.Left, k.Right, k.Wrap, k.Times, k.LineNumbers, k.Follow}, closing...),
	}
}

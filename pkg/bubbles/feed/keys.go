package feed

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a feed. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"first"`
	End          key.Binding `keymap:"bottom" help:"last"`
	// Retry repeats a failed fetch. The feed enables it only while a fetch
	// has failed, so help shows it only when it does something.
	Retry key.Binding `keymap:"global.refresh" help:"retry"`

	// Find opens the prompt of a find, which moves to the first row that
	// holds the text typed, and QuickFilter the prompt of a filter, which
	// shows only the loaded rows that hold it. Both match the text as it
	// is, a literal substring, ignoring case unless it has a capital. Next and Prev move between the rows a find
	// matches; the feed enables them only while one does, so help shows
	// them only when they work. The keys of the prompt itself are set with
	// [WithPromptKeys].
	Find        key.Binding `keymap:"find" help:"search"`
	QuickFilter key.Binding `keymap:"quick_filter" help:"quick filter"`
	Next        key.Binding `keymap:"next_match" help:"next match"`
	Prev        key.Binding `keymap:"prev_match" help:"prev match"`
}

// MarkKeys holds the key that marks rows. Only the lists whose rows can be
// marked have it, so it is not part of [KeyMap]: give it to a feed with
// [WithMarkKeys].
type MarkKeys struct {
	// Mark marks the row under the cursor, or unmarks it if it is marked.
	// A feed without a key for its items ([WithKey]), or without a key
	// bound to this, can't mark rows, and draws no cell for the mark.
	Mark key.Binding `keymap:"mark" help:"mark"`
}

// NewMarkKeys returns the key that look gives for marking rows. Without a
// key it is disabled.
func NewMarkKeys(look keymap.Lookup) MarkKeys {
	var k MarkKeys
	keymap.Fill(&k, look)
	return k
}

// NewKeyMap returns the key bindings that look gives the keys of, such as
// those of the context of the pane that shows the feed. An action without
// keys gives a disabled binding.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	// The feed enables the retry key while a fetch has failed.
	k.Retry.SetEnabled(false)
	k.Next.SetEnabled(false)
	k.Prev.SetEnabled(false)
	return k
}

// unbound is the keys of a feed made without a key map: none.
var unbound = keymap.Func(func(string) []string { return nil })

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Retry}
}

// FullHelp returns the bindings for the full help view. The keys of the
// prompt are the model's ([Model.FullHelp]).
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.Home, k.End, k.Retry},
		{k.Find, k.QuickFilter, k.Next, k.Prev},
	}
}

package diff

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a diff view. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"top"`
	End          key.Binding `keymap:"bottom" help:"bottom"`
	// Left and Right scroll sideways; lines are never wrapped.
	Left  key.Binding `keymap:"left" help:"left"`
	Right key.Binding `keymap:"right" help:"right"`
	// NextFile moves to the header of the next file, and PrevFile to the
	// header of the file the cursor is in, or of the one before it when
	// the cursor is on a header already.
	NextFile key.Binding `keymap:"next_file" help:"next file"`
	PrevFile key.Binding `keymap:"prev_file" help:"prev file"`
	// NextHunk moves to the next hunk header, in the next file with one
	// when the cursor is past the last hunk of its file, and PrevHunk to the
	// header of the hunk the cursor is in, or of the one before it.
	NextHunk key.Binding `keymap:"next_hunk" help:"next hunk"`
	PrevHunk key.Binding `keymap:"prev_hunk" help:"prev hunk"`
	// Fold folds or unfolds the file whose header the cursor is on.
	Fold key.Binding `keymap:"global.select" help:"fold"`
	// Retry repeats a failed fetch. The view enables it only while a fetch
	// has failed, so help shows it only when it does something.
	Retry key.Binding `keymap:"global.refresh" help:"retry"`
}

// NewKeyMap returns the key bindings that look gives the keys of, such as
// those of the context of the pane that shows the diff. An action without
// keys gives a disabled binding.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	// The view enables the retry key while a fetch has failed.
	k.Retry.SetEnabled(false)
	return k
}

// unbound is the keys of a view made without a key map: none.
var unbound = keymap.Func(func(string) []string { return nil })

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.NextFile, k.NextHunk, k.Fold, k.Retry}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End},
		{k.NextFile, k.PrevFile, k.NextHunk, k.PrevHunk, k.Fold, k.Retry},
	}
}

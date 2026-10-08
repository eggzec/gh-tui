package files

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// KeyMap holds the keys of the section. It implements help.KeyMap.
type KeyMap struct {
	Open    key.Binding
	Refresh key.Binding
	// ResetBase shows the head of the default branch again, once the
	// files show another base.
	ResetBase key.Binding
	// Tree moves through the files and expands directories.
	Tree tree.KeyMap
}

// ctxPane is the context of the keys of the file tree.
const ctxPane = "files"

func newKeyMap(keys config.Keymap) KeyMap {
	files := ui.In(keys, ctxPane)
	tk := tree.NewKeyMap(files)
	// A preview opens a file, and a directory folds.
	tk.Open = files.Binding("global.select", "preview")
	return KeyMap{
		Open:      files.Binding("global.open", "open"),
		Refresh:   files.Binding("global.refresh", "refresh"),
		ResetBase: files.Binding("reset_base", "back to head"),
		Tree:      tk,
	}
}

// own returns the keys of the section itself, in the order it matches
// them.
func (k KeyMap) own() []key.Binding {
	return []key.Binding{k.Refresh, k.Open, k.ResetBase}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tree.Up, k.Tree.Down, k.Tree.Expand, k.Tree.Collapse, k.Tree.ToggleAll, k.Tree.Open, k.Open, k.Refresh, k.ResetBase}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return append(k.Tree.FullHelp(), k.own())
}

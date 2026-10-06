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
	tk := tree.DefaultKeyMap()
	tk.Expand = files.Binding("expand", "expand")
	// The arrows and h stay alongside the configured keys, as l and → do
	// for Right.
	tk.Collapse = withKeys(files.Binding("collapse", "collapse"), "collapse", "←/h", "left", "h")
	tk.ExpandAll = files.Binding("expand_all", "expand all")
	tk.CollapseAll = files.Binding("collapse_all", "collapse all")
	tk.Open = files.Binding("global.select", "preview")
	return KeyMap{
		Open:      files.Binding("global.open", "open"),
		Refresh:   files.Binding("global.refresh", "refresh"),
		ResetBase: files.Binding("reset_base", "back to head"),
		Tree:      tk,
	}
}

// withKeys adds keys, labelled label in help, to b.
func withKeys(b key.Binding, desc, label string, keys ...string) key.Binding {
	if !b.Enabled() {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, desc))
	}
	b.SetKeys(append(b.Keys(), keys...)...)
	b.SetHelp(b.Help().Key+"/"+label, desc)
	return b
}

// own returns the keys of the section itself, in the order it matches
// them.
func (k KeyMap) own() []key.Binding {
	return []key.Binding{k.Refresh, k.Open, k.ResetBase}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tree.Up, k.Tree.Down, k.Tree.Right, k.Tree.Collapse, k.Tree.Open, k.Open, k.Refresh, k.ResetBase}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return append(k.Tree.FullHelp(), k.own())
}

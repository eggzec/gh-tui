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

func newKeyMap(keys map[string][]string) KeyMap {
	tk := tree.DefaultKeyMap()
	tk.Expand = ui.Binding(keys, config.ActionExpand, "expand")
	// The arrows and h stay alongside the configured keys, as l and → do
	// for Right.
	tk.Collapse = withKeys(ui.Binding(keys, config.ActionCollapse, "collapse"), "collapse", "←/h", "left", "h")
	tk.ExpandAll = ui.Binding(keys, config.ActionExpandAll, "expand all")
	tk.CollapseAll = ui.Binding(keys, config.ActionCollapseAll, "collapse all")
	tk.Open = ui.Binding(keys, config.ActionSelect, "preview")
	return KeyMap{
		Open:      ui.Binding(keys, config.ActionOpen, "open"),
		Refresh:   ui.Binding(keys, config.ActionRefresh, "refresh"),
		ResetBase: ui.Binding(keys, config.ActionResetBase, "back to head"),
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

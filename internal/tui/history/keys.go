package history

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
)

// KeyMap holds the keys of the modal. The panes share the keys that move
// through a list, which the graph has too.
type KeyMap struct {
	// Next and Prev move the focus between the panes, and Panes focus the
	// pane with that number.
	Next  key.Binding
	Prev  key.Binding
	Panes [numPanes]key.Binding
	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding
	// Select shows the graph of a branch, what a commit changed, or the
	// patch of a file, in the pane after.
	Select key.Binding
	// Back shows every pane again while one is zoomed, or steps back to
	// the pane before, and closes the modal from the branches.
	Back key.Binding
	// Zoom shows the focused pane alone, or every pane again.
	Zoom key.Binding
	// Filter narrows the branches as the user types.
	Filter key.Binding
	// UseAsBase shows the files at the branch or commit under the cursor
	// of the branches, graph of commits and the commit's files, with the
	// key of each, and ResetBase at the head of the default branch again.
	UseAsBase, graphBase, filesBase key.Binding
	ResetBase                       key.Binding
	// Open opens the branch, commit or file on GitHub.
	Open key.Binding
	// Retry reads again what failed to load.
	Retry key.Binding
	// List moves through the branches and the files, and Graph through
	// the commits.
	List  graph.KeyMap
	Graph graph.KeyMap
}

// The contexts of the keys of the modal: its own, and one for each pane.
const (
	ctxModal    = "history"
	ctxBranches = "history_branches"
	ctxGraph    = "history_graph"
	ctxFiles    = "history_files"
)

func newKeyMap(keys config.Keymap) KeyMap {
	modal, branches, graphCtx, files := ui.In(keys, ctxModal), ui.In(keys, ctxBranches), ui.In(keys, ctxGraph), ui.In(keys, ctxFiles)
	list := graph.DefaultKeyMap()
	g := graph.DefaultKeyMap()
	g.Choose = graphCtx.Binding("global.select", "open")
	g.Retry = graphCtx.Binding("global.refresh", "retry")
	// The graph enables its retry key while a fetch has failed.
	g.Retry.SetEnabled(false)
	k := KeyMap{
		Next:      modal.Binding("global.next_pane", "pane"),
		Prev:      modal.Binding("global.prev_pane", "previous pane"),
		Select:    modal.Binding("global.select", "open"),
		Back:      modal.Binding("global.dismiss", "back"),
		Zoom:      modal.Binding("global.zoom", "zoom"),
		Filter:    branches.Binding("filter", "filter"),
		UseAsBase: branches.Binding("base", "use as base"),
		graphBase: graphCtx.Binding("base", "use as base"),
		filesBase: files.Binding("base", "use as base"),
		ResetBase: modal.Binding("reset_base", "back to head"),
		Open:      modal.Binding("global.open", "browser"),
		Retry:     modal.Binding("global.refresh", "retry"),
		List:      list,
		Graph:     g,
	}
	for i, a := range [numPanes]string{"global.pane_1", "global.pane_2", "global.pane_3"} {
		k.Panes[i] = modal.Binding(a, paneTitles[i])
	}
	k.Jump = ui.Jump(k.Panes[:]...)
	return k
}

// paneTitles names the panes in help.
var paneTitles = [numPanes]string{"branches", "graph", "commit"}

// focusOf returns the pane that msg focuses, or -1.
func (k KeyMap) focusOf(msg tea.KeyPressMsg) pane {
	for i, b := range k.Panes {
		if key.Matches(msg, b) {
			return pane(i)
		}
	}
	return -1
}

// base returns the key that shows the files at what is under the cursor
// of the pane p.
func (k KeyMap) base(p pane) key.Binding {
	switch p {
	case graphPane:
		return k.graphBase
	case commitPane:
		return k.filesBase
	case branchPane:
	}
	return k.UseAsBase
}

// paneContext returns the context of the keys of the pane p, and of the
// patch of a file in the commit pane if patch.
func paneContext(p pane, patch bool) string {
	switch {
	case p == graphPane:
		return ctxGraph
	case p == commitPane && patch:
		return "history_patch"
	case p == commitPane:
		return ctxFiles
	}
	return ctxBranches
}

// own returns the keys of the modal itself, in the order it matches them.
func (k KeyMap) own() []key.Binding {
	return []key.Binding{k.Next, k.Prev, k.Jump, k.Open, k.ResetBase, k.Zoom, k.Back, k.UseAsBase, k.Select, k.Filter, k.Retry}
}

// screen returns the keys of the modal that work in every pane, and the
// keys of the pane that has the focus, in the order the modal matches them.
func (k KeyMap) screen() []key.Binding {
	return []key.Binding{k.Next, k.Prev, k.Jump, k.Open, k.ResetBase, k.Zoom, k.Back, k.Retry}
}

// pane returns the keys of the focused pane that the modal matches, with
// the keys of the pane worth a hint.
func (k KeyMap) pane(p pane) (keys, short []key.Binding) {
	base := k.base(p)
	if p == branchPane {
		return []key.Binding{base, k.Select, k.Filter}, []key.Binding{k.Select, k.Filter, base}
	}
	return []key.Binding{base, k.Select}, []key.Binding{k.Select, base}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Filter, k.UseAsBase, k.Open, k.Retry, k.Next, k.Zoom, k.Back}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return slices.Concat([][]key.Binding{k.own()}, k.List.FullHelp(), k.Graph.FullHelp())
}

// named returns b described as desc.
func named(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

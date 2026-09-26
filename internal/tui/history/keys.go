package history

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
)

// KeyMap holds the keys of the modal. The panes share the keys that move
// through a list, which the graph has too.
type KeyMap struct {
	// Next and Prev move the focus between the panes.
	Next key.Binding
	Prev key.Binding
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
	// UseAsBase shows the files at the branch or commit under the cursor,
	// and ResetBase at the head of the default branch again.
	UseAsBase key.Binding
	ResetBase key.Binding
	// Open opens the branch, commit or file on GitHub.
	Open key.Binding
	// Retry reads again what failed to load.
	Retry key.Binding
	// List moves through the branches and the files, and Graph through
	// the commits.
	List  graph.KeyMap
	Graph graph.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	list := graph.DefaultKeyMap()
	g := graph.DefaultKeyMap()
	g.Choose = ui.Binding(keys, config.ActionSelect, "open")
	g.Retry = ui.Binding(keys, config.ActionRefresh, "retry")
	// The graph enables its retry key while a fetch has failed.
	g.Retry.SetEnabled(false)
	return KeyMap{
		Next:      ui.Binding(keys, config.ActionNextTab, "pane"),
		Prev:      ui.Binding(keys, config.ActionPrevTab, "previous pane"),
		Select:    ui.Binding(keys, config.ActionSelect, "open"),
		Back:      ui.Binding(keys, config.ActionBack, "back"),
		Zoom:      ui.Binding(keys, config.ActionZoom, "zoom"),
		Filter:    ui.Binding(keys, config.ActionSearch, "filter"),
		UseAsBase: ui.Binding(keys, config.ActionUseAsBase, "use as base"),
		ResetBase: ui.Binding(keys, config.ActionResetBase, "back to head"),
		Open:      ui.Binding(keys, config.ActionOpen, "browser"),
		Retry:     ui.Binding(keys, config.ActionRefresh, "retry"),
		List:      list,
		Graph:     g,
	}
}

package checktree

import (
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// FoldRows returns rows as the rows of a foldtree, for the hosts that still
// draw their tree with one: a branch is a fold with an arrow.
func FoldRows(rows []treeview.Node) []foldtree.Row {
	out := make([]foldtree.Row, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = foldtree.Row{
			ID: r.ID, Depth: r.Depth, Lines: r.Lines, Marker: r.Marker, Right: r.Right,
			Fold: r.Branch, Arrow: r.Branch, Closed: r.Closed, Action: r.Action,
			Skip: r.Skip, Kind: r.Kind, Inline: r.Inline,
		}
	}
	return out
}

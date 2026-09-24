package tree_test

import (
	"context"
	"path"
	"strings"

	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

func Example() {
	// The tui adapts a service read of a directory to Children; here the
	// tree is a map from directory to entries.
	dirs := map[string][]string{
		"":         {"cmd/", "internal/", "go.mod"},
		"cmd":      {"main.go"},
		"internal": {"core/", "tui/"},
	}
	children := func(_ context.Context, parent tree.Node) ([]tree.Node, error) {
		nodes := make([]tree.Node, 0, len(dirs[parent.ID]))
		for _, name := range dirs[parent.ID] {
			dir := strings.HasSuffix(name, "/")
			name = strings.TrimSuffix(name, "/")
			nodes = append(nodes, tree.Node{ID: path.Join(parent.ID, name), Name: name, Branch: dir})
		}
		return nodes, nil
	}

	m := tree.New(children,
		tree.WithSize(40, 20),
		tree.WithEmptyText("This repository is empty."),
		// Expand-all loads at most 500 nodes, 3 levels deep, 2 at a time.
		tree.WithExpandAllLimits(500, 3),
		tree.WithMaxLoads(2),
	)
	m.Focus()

	// Return m.Init() from the parent's Init and forward messages to
	// m.Update. Enter on a leaf sends a tree.OpenMsg whose ID is m.ID(), to
	// show a preview of msg.Node; call m.Reload() when the tree changes.
	_ = m.Init()
}

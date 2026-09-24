package graph_test

import (
	"context"

	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
)

func Example() {
	// The tui adapts a service read of a branch's commits to Fetch; here the
	// history is one chunk.
	history := []graph.Commit{
		{ID: "c3", Parents: []string{"c2", "f1"}, Short: "c3", Title: "Merge pull request #1", Detail: "octocat", Right: "1h"},
		{ID: "f1", Parents: []string{"c1"}, Short: "f1", Title: "feat: add the graph", Detail: "hubot", Right: "2h"},
		{ID: "c2", Parents: []string{"c1"}, Short: "c2", Title: "fix: typo", Detail: "octocat", Right: "3h"},
		{ID: "c1", Short: "c1", Title: "Initial commit", Detail: "octocat", Right: "1d"},
	}
	fetch := func(_ context.Context, _ string) ([]graph.Commit, string, error) {
		return history, "", nil
	}

	m := graph.New(fetch,
		graph.WithSize(40, 20),
		graph.WithMaxLanes(6),
		graph.WithEmptyText("No commits on this branch."),
	)
	m.Focus()

	// Return m.Init() from the parent's Init and forward messages to
	// m.Update. A graph.SelectMsg whose ID is m.ID() names the commit under
	// the cursor, to show its diff; enter sends a graph.ChosenMsg. Call
	// m.Reset(fetch) with the fetch of another branch to show its history.
	_ = m.Init()
}

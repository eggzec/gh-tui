package finder_test

import (
	"context"

	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
)

func Example() {
	// The tui adapts a service to Load, such as the listing of a
	// repository; here it returns fixed paths.
	load := func(context.Context) (finder.Listing, error) {
		return finder.Listing{Items: []finder.Item{
			{Path: "cmd/app/main.go", Detail: "1.2K"},
			{Path: "internal/tui/model.go", Detail: "8.4K"},
			{Path: "README.md", Detail: "3K"},
		}}, nil
	}
	m := finder.New(load,
		finder.WithSize(60, 16),
		finder.WithRecent([]string{"README.md"}),
	)
	m.Focus()

	// Return m.Init() when the finder opens, forward messages to m.Update
	// while it is open, and close it on finder.ChosenMsg or
	// finder.CancelMsg with m.ID().
	_ = m.Init()
}

package picker_test

import (
	"context"
	"strings"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

type repo struct {
	Owner, Name string
}

func Example() {
	repos := []repo{{"eggzec", "gh-tui"}, {"octo-org", "dotfiles"}}

	// The tui adapts a search service to Search; here it filters a slice.
	// An empty query lists something to start from.
	search := func(_ context.Context, q picker.Query) ([]picker.Item, error) {
		var items []picker.Item
		for _, r := range repos {
			name := r.Owner + "/" + r.Name
			if strings.Contains(name, q.Text) {
				items = append(items, picker.Item{Kind: "Repositories", Title: name, Value: r})
			}
		}
		return items, nil
	}

	m := picker.New(search,
		picker.WithSize(60, 16),
		picker.WithScopes("Repositories", "Issues", "Pull requests"),
		picker.WithPlaceholder("Search GitHub"),
	)
	_ = m.Focus()

	// Return m.Init() when the picker opens, forward messages to m.Update
	// while it is open, and close it on picker.ChosenMsg or
	// picker.CancelMsg with m.ID(). ChosenMsg.Item.Value holds the repo.
	_ = m.Init()
}

func Example_local() {
	// Without a Search function, the picker filters fixed items with fuzzy
	// matching.
	m := picker.New(nil, picker.WithItems([]picker.Item{
		{Kind: "Sections", Title: "Pull requests"},
		{Kind: "Sections", Title: "Issues"},
		{Kind: "Sections", Title: "Notifications"},
	}))
	_ = m.Focus()
	_ = m.View()
}

package feed_test

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

type pull struct {
	Number int
	Title  string
}

func Example() {
	// The tui adapts a paged service read to Fetch; here the pages are
	// generated.
	fetch := func(_ context.Context, cursor string) ([]pull, string, error) {
		start, _ := strconv.Atoi(cursor)
		var page []pull
		for n := start; n < start+3; n++ {
			page = append(page, pull{Number: n + 1, Title: "Fix things"})
		}
		return page, strconv.Itoa(start + 3), nil
	}
	render := func(p pull, _ bool, _ int) string {
		return fmt.Sprintf("#%d %s", p.Number, p.Title)
	}

	// Options need no type arguments; WithKey infers its type from the
	// function.
	m := feed.New(fetch, render,
		feed.WithSize(80, 20),
		feed.WithKey(func(p pull) string { return strconv.Itoa(p.Number) }),
		feed.WithEmptyText("No pull requests match. Press / to change the filter."),
	)
	m.Focus()

	// Return m.Init() from the parent's Init, forward messages to
	// m.Update, and call m.Reload() when a sync event arrives.
	_ = m.Init()
}

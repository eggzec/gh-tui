package finder

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostilePaths(t *testing.T) {
	h := termtexttest.Hostile
	load := func(context.Context) (Listing, error) {
		return Listing{Items: []Item{{Path: "dir/" + h, Detail: h}}, Note: h}, nil
	}
	for _, w := range []int{30, 60, 200} {
		m := newKeyed(t, load, WithSize(w, 6))
		m.Focus()
		m = run(t, m, m.Init())
		termtexttest.AssertClean(t, m.View(), w)
	}
}

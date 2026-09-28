package tree

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileNames(t *testing.T) {
	h := termtexttest.Hostile
	children := func(context.Context, Node) ([]Node, error) {
		return []Node{{ID: "a", Name: h, Branch: true}, {ID: "b", Name: h, Detail: h}}, nil
	}
	for _, w := range []int{30, 200} {
		m := New(children, WithSize(w, 4), WithFocused(true))
		m = run(t, m, m.Init())
		termtexttest.AssertClean(t, m.View(), w)
	}
}

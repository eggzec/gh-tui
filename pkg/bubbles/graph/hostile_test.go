package graph

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileCommits(t *testing.T) {
	h := termtexttest.Hostile
	fetch := func(context.Context, string) ([]Commit, string, error) {
		return []Commit{{ID: "1", Short: h, Title: h, Detail: h, Right: h}}, "", nil
	}
	for _, w := range []int{30, 200} {
		m := New(fetch, WithSize(w, 3))
		m, _ = run(t, m, m.Init())
		termtexttest.AssertClean(t, m.View(), w)
	}
}

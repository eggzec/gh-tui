package search

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// The titles, labels, descriptions, paths and code of results are text
// from GitHub, which the rows draw.
func TestRowsCleanHostileHits(t *testing.T) {
	h := termtexttest.Hostile
	s := newSection(t, newFake(), 140, 38)
	is := issue(core.SearchIssues, "cli/cli", 1, h, core.StateOpen, false)
	is.Issue.Labels = []core.Label{{Name: h, Color: "d73a4a"}}
	r := repo("cli", "cli", h, "Go", 38000, 0)
	code := core.CodeHit{
		Repo: core.RepoRef{Owner: "cli", Name: "cli"}, Path: "dir/" + h,
		Fragments: []core.Fragment{{Text: "x := " + h + "\nnext", Matches: [][2]int{{0, 1}, {5, 10}}}},
	}
	for _, w := range []int{40, 80, 200} {
		termtexttest.AssertClean(t, s.renderHit(is, false, w), w)
		termtexttest.AssertClean(t, s.renderHit(r, true, w), w)
		termtexttest.AssertClean(t, s.renderCode(code, false, w), w)
	}
}

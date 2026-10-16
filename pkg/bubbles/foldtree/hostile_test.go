package foldtree

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// The text of a row comes from the caller, which cleans what GitHub sent;
// the tree must keep it clean at any width, through its guides, markers,
// right-aligned parts, body lines and cut text.
func TestViewKeepsCleanedHostileTextClean(t *testing.T) {
	h := termtext.Clean(termtexttest.Hostile, 4)
	rows := []Row{
		{ID: termtexttest.Hostile, Lines: []string{h, h + "\t" + h}, Marker: h, Right: h, Fold: true, Closed: true, Arrow: true},
		{ID: h, Depth: 1, Lines: []string{h, h}, Right: h},
		{ID: "x", Depth: 2, Lines: []string{h}, Marker: h, Fold: true},
		{ID: h + "2", Depth: 0, Lines: []string{h, h}, Fold: true},
	}
	for _, w := range []int{1, 4, 10, 30, 200} {
		m := newTree(rows, WithSize(w, 8))
		termtexttest.AssertClean(t, m.View(), w)
		m, _ = keys(t, m, "*", "j", "j", "*", "G")
		termtexttest.AssertClean(t, m.View(), w)
	}
}

// A newline inside a line would add a row to the screen, and a tab would
// shift the columns after it: the tree takes neither through.
func TestViewSplitsNewlinesAndTabs(t *testing.T) {
	m := newTree([]Row{{ID: "a", Lines: []string{"one\ntwo\tx", "three"}}}, WithSize(20, 6))
	termtexttest.AssertClean(t, m.View(), 20)
}

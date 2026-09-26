package dashboard

import (
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Sizes of a card of the pinned pane: its lines, the fewest and most cells
// it may get, and the gap between two cards. The most keeps a lone card
// from stretching its lines across a wide pane.
const (
	cardHeight   = 4
	minCardWidth = 26
	maxCardWidth = 72
	cardGap      = 2
)

// card is a repository of the pinned pane.
type card struct {
	repo core.Repo
	// here marks the repository of the current directory.
	here bool
}

// cards is the pinned pane: the repository of the current directory, if
// any, then the pinned ones, laid out in a grid of rows.
type cards struct {
	here     core.RepoRef
	hereRepo core.Repo
	items    []card
	sel      int
	// cols is how many cards a row holds, and rows how many rows show.
	cols, rows int
	top        int
}

// set lists pinned after the repository of the current directory, which
// takes the card of its pin if it has one.
func (c *cards) set(pinned []core.Repo) {
	var prev core.RepoRef
	if c.sel < len(c.items) {
		prev = c.items[c.sel].repo.Ref
	}
	items := make([]card, 0, len(pinned)+1)
	if c.here != (core.RepoRef{}) {
		r := c.hereRepo
		if i := slices.IndexFunc(pinned, func(p core.Repo) bool { return sameRef(p.Ref, c.here) }); i >= 0 {
			r = pinned[i]
		}
		if r.Ref == (core.RepoRef{}) {
			r.Ref = c.here
		}
		items = append(items, card{repo: r, here: true})
	}
	for i := range pinned {
		if c.here == (core.RepoRef{}) || !sameRef(pinned[i].Ref, c.here) {
			items = append(items, card{repo: pinned[i]})
		}
	}
	c.items = items
	c.sel = max(slices.IndexFunc(items, func(it card) bool { return it.repo.Ref == prev }), 0)
	c.scroll()
}

// selected returns the card under the cursor.
func (c *cards) selected() (card, bool) {
	if c.sel >= len(c.items) {
		return card{}, false
	}
	return c.items[c.sel], true
}

// resize lays the cards out in width by height cells.
func (c *cards) resize(width, height int) {
	c.cols = max((width+cardGap)/(minCardWidth+cardGap), 1)
	c.rows = max((height+1)/(cardHeight+1), 1)
	c.scroll()
}

// cardWidth is the width of the card in column col of a pane of width
// cells. A row shares its width among the cards it holds, or among all of
// them when they are fewer, so that a few pins don't leave most of the
// pane empty. The last column takes what the division leaves over, so the
// row ends flush with the pane.
func (c *cards) cardWidth(width, col int) int {
	n := max(min(c.cols, len(c.items)), 1)
	w := min(max((width-cardGap*(n-1))/n, 1), maxCardWidth)
	if col == n-1 {
		w = min(max(width-col*(w+cardGap), 1), maxCardWidth)
	}
	return w
}

// move moves the cursor by delta cards, and stays on the first or last.
func (c *cards) move(delta int) {
	if len(c.items) == 0 {
		return
	}
	c.sel = min(max(c.sel+delta, 0), len(c.items)-1)
	c.scroll()
}

// scroll shows the row of the cursor.
func (c *cards) scroll() {
	if c.cols == 0 {
		return
	}
	row := c.sel / c.cols
	if row < c.top {
		c.top = row
	}
	if row >= c.top+c.rows {
		c.top = row - c.rows + 1
	}
	last := max((len(c.items)+c.cols-1)/c.cols-c.rows, 0)
	c.top = min(max(c.top, 0), last)
}

// pages reports the first row on view and the rows in all, for the label,
// or zeros when every row is on view.
func (c *cards) pages() (at, of int) {
	if c.cols == 0 {
		return 0, 0
	}
	of = (len(c.items) + c.cols - 1) / c.cols
	if of <= c.rows {
		return 0, 0
	}
	return c.top + 1, of
}

// sameRef reports whether a and b name the same repository, which GitHub
// matches regardless of case.
func sameRef(a, b core.RepoRef) bool {
	return strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Name, b.Name)
}

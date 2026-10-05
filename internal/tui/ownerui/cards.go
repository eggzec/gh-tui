package ownerui

import (
	"slices"

	"github.com/eggzec/gh-tui/internal/core"
)

// Sizes of a card: its lines, the fewest and most cells it may get, and
// the gap between two cards. The most keeps a lone card from stretching its
// lines across a wide pane.
const (
	CardHeight   = 4
	MinCardWidth = 26
	MaxCardWidth = 72
	CardGap      = 2
)

// Card is a pinned repository.
type Card struct {
	Repo core.Repo
	// Here marks the repository of the current directory.
	Here bool
}

// Cards are pinned repositories laid out in a grid of rows: the
// repository of the current directory, if any, then the pinned ones.
type Cards struct {
	Here     core.RepoRef
	HereRepo core.Repo
	Items    []Card
	Sel      int
	// Cols is how many cards a row holds, and Rows how many rows show.
	Cols, Rows int
	Top        int
}

// Set lists pinned after the repository of the current directory, which
// takes the card of its pin if it has one, and reports whether that
// changed the repositories listed.
func (c *Cards) Set(pinned []core.Repo) bool {
	var prev core.RepoRef
	if c.Sel < len(c.Items) {
		prev = c.Items[c.Sel].Repo.Ref
	}
	items := make([]Card, 0, len(pinned)+1)
	if c.Here != (core.RepoRef{}) {
		r := c.HereRepo
		if i := slices.IndexFunc(pinned, func(p core.Repo) bool { return p.Ref.Same(c.Here) }); i >= 0 {
			r = pinned[i]
		}
		if r.Ref == (core.RepoRef{}) {
			r.Ref = c.Here
		}
		items = append(items, Card{Repo: r, Here: true})
	}
	for i := range pinned {
		if c.Here == (core.RepoRef{}) || !pinned[i].Ref.Same(c.Here) {
			items = append(items, Card{Repo: pinned[i]})
		}
	}
	changed := !slices.EqualFunc(c.Items, items, func(a, b Card) bool { return a.Repo.Ref == b.Repo.Ref })
	c.Items = items
	c.Sel = max(slices.IndexFunc(items, func(it Card) bool { return it.Repo.Ref.Same(prev) }), 0)
	c.scroll()
	return changed
}

// Selected returns the card under the cursor.
func (c *Cards) Selected() (Card, bool) {
	if c.Sel >= len(c.Items) {
		return Card{}, false
	}
	return c.Items[c.Sel], true
}

// Resize lays the cards out in width by height cells.
func (c *Cards) Resize(width, height int) {
	c.Cols = max((width+CardGap)/(MinCardWidth+CardGap), 1)
	c.Rows = max((height+1)/(CardHeight+1), 1)
	c.scroll()
}

// CardWidth is the width of the card in column col of a pane of width
// cells. A row shares its width among the cards it holds, or among all of
// them when they are fewer, so that a few pins don't leave most of the
// pane empty. The last column takes what the division leaves over, so the
// row ends flush with the pane.
func (c *Cards) CardWidth(width, col int) int {
	n := max(min(c.Cols, len(c.Items)), 1)
	w := min(max((width-CardGap*(n-1))/n, 1), MaxCardWidth)
	if col == n-1 {
		w = min(max(width-col*(w+CardGap), 1), MaxCardWidth)
	}
	return w
}

// Move moves the cursor by delta cards, and stays on the first or last.
func (c *Cards) Move(delta int) {
	if len(c.Items) == 0 {
		return
	}
	c.Sel = min(max(c.Sel+delta, 0), len(c.Items)-1)
	c.scroll()
}

// scroll shows the row of the cursor.
func (c *Cards) scroll() {
	if c.Cols == 0 {
		return
	}
	row := c.Sel / c.Cols
	if row < c.Top {
		c.Top = row
	}
	if row >= c.Top+c.Rows {
		c.Top = row - c.Rows + 1
	}
	last := max((len(c.Items)+c.Cols-1)/c.Cols-c.Rows, 0)
	c.Top = min(max(c.Top, 0), last)
}

// Pages reports the first row on view and the rows in all, for the label,
// or zeros when every row is on view.
func (c *Cards) Pages() (at, of int) {
	if c.Cols == 0 {
		return 0, 0
	}
	of = (len(c.Items) + c.Cols - 1) / c.Cols
	if of <= c.Rows {
		return 0, 0
	}
	return c.Top + 1, of
}

// Package statusbar draws one line of items, such as key hints on the
// left and the state of a connection on the right, that gives way as the
// line narrows: items shrink to shorter forms, then leave, in the order
// their ranks say.
//
// The parent sets the items with [Model.SetItems] and the width with
// [Model.SetWidth], and puts [Model.View] where the bar goes. The bar
// takes no messages, and lays itself out whenever what it shows changes.
package statusbar

import (
	"slices"

	"github.com/charmbracelet/x/ansi"
)

// Item is one thing the bar shows.
type Item struct {
	// Forms are the ways to show the item, styled, widest first. The bar
	// shows the first that fits, and leaves the item out when none does.
	// An empty form is skipped.
	Forms []string
	// Rank says when the item gives way as the bar narrows: the items of
	// the lowest rank give way first, the last of them first, each
	// shrinking to its next form and then leaving the bar, before an item
	// of a higher rank gives way at all. The left items come before the
	// right ones.
	Rank int
}

// The spacing of the bar: a cell at each edge, two between the items on
// the left, a separator between those on the right, and a gap of at
// least two cells between the sides.
const (
	edge    = 1
	leftSep = "  "
	gap     = 2
)

// Model is a status bar.
type Model struct {
	width       int
	left, right []Item
	styles      Styles
	// sep is the separator of the items on the right, rendered.
	sep string
	// widths holds the width of each form of each item, the left ones
	// first, and forms the form each shows, len(Forms) for none.
	widths [][]int
	forms  []int
	view   string
}

// New returns a bar with no items.
func New(opts ...Option) Model {
	m := Model{}
	m.SetStyles(DefaultStyles(true))
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// SetWidth sets the width of the bar.
func (m *Model) SetWidth(width int) {
	if width == m.width {
		return
	}
	m.width = max(width, 0)
	m.layout()
}

// Width returns the width of the bar.
func (m Model) Width() int { return m.width }

// SetItems sets the items on the left and on the right of the bar, in the
// order they are shown.
func (m *Model) SetItems(left, right []Item) {
	m.left, m.right = clean(left), clean(right)
	m.widths = make([][]int, 0, len(m.left)+len(m.right))
	for _, it := range slices.Concat(m.left, m.right) {
		w := make([]int, len(it.Forms))
		for i, f := range it.Forms {
			w[i] = ansi.StringWidth(f)
		}
		m.widths = append(m.widths, w)
	}
	m.layout()
}

// clean returns items without their empty forms.
func clean(items []Item) []Item {
	out := make([]Item, len(items))
	for i, it := range items {
		out[i].Rank = it.Rank
		for _, f := range it.Forms {
			if f != "" {
				out[i].Forms = append(out[i].Forms, f)
			}
		}
	}
	return out
}

// Shown returns the form each item shows, the left items first, as an
// index into its Forms, or -1 for an item left out.
func (m Model) Shown() []int {
	out := make([]int, len(m.forms))
	for i, f := range m.forms {
		out[i] = f
		if f >= len(m.widths[i]) {
			out[i] = -1
		}
	}
	return out
}

package statusbar

import (
	"cmp"
	"slices"
	"strings"
)

// sepWidth is the width of the separator on the right.
const sepWidth = 3

// View returns the bar, exactly as wide as its width, or "" when it has
// no width.
func (m Model) View() string { return m.view }

// layout picks the form of each item, giving way in rank order until the
// bar fits, and renders it.
func (m *Model) layout() {
	n := len(m.widths)
	m.forms = make([]int, n)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	rank := func(i int) int {
		if i < len(m.left) {
			return m.left[i].Rank
		}
		return m.right[i-len(m.left)].Rank
	}
	// The lowest rank first, and the last item of a rank first.
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(b, a)) })
	for _, i := range order {
		for m.need() > m.width && m.forms[i] < len(m.widths[i]) {
			m.forms[i]++
		}
	}
	m.render()
}

// need returns the width the items take in their forms now.
func (m *Model) need() int {
	lw, ln := m.side(0, len(m.left))
	rw, rn := m.side(len(m.left), len(m.widths))
	if ln == 0 && rn == 0 {
		return 0
	}
	w := 2*edge + lw + len(leftSep)*max(ln-1, 0) + rw + sepWidth*max(rn-1, 0)
	if ln > 0 && rn > 0 {
		w += gap
	}
	return w
}

// side returns the width of the items from to to in their forms now, and
// how many of them show.
func (m *Model) side(from, to int) (width, shown int) {
	for i := from; i < to; i++ {
		if f := m.forms[i]; f < len(m.widths[i]) {
			width += m.widths[i][f]
			shown++
		}
	}
	return width, shown
}

func (m *Model) render() {
	if m.width <= 0 {
		m.view = ""
		return
	}
	need := m.need()
	if need == 0 || need > m.width {
		m.view = strings.Repeat(" ", m.width)
		return
	}
	join := func(items []Item, from int, sep string) string {
		var parts []string
		for i, it := range items {
			if f := m.forms[from+i]; f < len(it.Forms) {
				parts = append(parts, it.Forms[f])
			}
		}
		return strings.Join(parts, sep)
	}
	left, right := join(m.left, 0, leftSep), join(m.right, len(m.left), m.sep)
	pad := m.width - need
	if left != "" && right != "" {
		pad += gap
	}
	m.view = " " + left + strings.Repeat(" ", pad) + right + " "
}

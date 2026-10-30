package pager

import "slices"

// A host that folds rows tells the pager which of them are shown with
// [Model.SetVisible]. The mask is synchronous and the filter is not, so they
// are two sources: pvis is what the filter and squeeze picked, mask is what
// the host shows, and vis, the lines shown, is the rows both of them show.
// A late filter result replaces pvis and composes with the mask as it is,
// and a new mask composes with the pvis as it is; neither cancels the other.

// WithReveal gives the pager a hook that a search calls before it lands on a
// row the mask hides: the host opens the fold that holds the row and returns
// the new mask, with changed set. With the hook, a search finds matches in
// the rows the mask hides too; without it, a search is over the rows shown.
// A hook that returns changed false leaves the match hidden: the search does
// not jump to it, and says so as it does for a pattern not found.
func WithReveal(reveal func(row int) (vis []int32, changed bool)) Option {
	return func(s *settings) { s.reveal = reveal }
}

// SetVisible sets which rows the pager shows at once: vis lists their indexes
// in the [Rows] in ascending order, and nil shows every row again. anchor
// returns, for a row the mask hides, the row shown that stands in for it,
// such as the header of its fold; the window and the cursor go there when
// they were on a hidden row. The mask holds for the document shown, and
// composes with the filter and with squeeze: a row is shown only when all of
// them show it. Rows appended with [Model.Append] after the mask was set are
// not shown until SetVisible is called again. A search without the
// [WithReveal] hook is over the rows shown when it runs, so run it again after
// the mask changes. The pager keeps vis without copying it, as it keeps the
// mask the reveal hook returns, since a mask can list every row of a long
// log: the host must not change either after it hands it over.
func (m *Model) SetVisible(vis []int32, anchor func(hidden int) int) {
	top := m.topLine()
	if vis == nil {
		m.mask, m.maskAnchor = nil, nil
	} else {
		m.mask, m.maskAnchor = vis, anchor
	}
	m.applyMask(top)
}

// applyMask composes the lines shown with the mask, and keeps the window on
// line top, or the row that stands in for it, and the cursor on a line shown.
func (m *Model) applyMask(top int) {
	m.compose()
	if m.cursoring {
		// Before the clamp, which would take it to the next row shown.
		m.cursor = m.stand(m.cursor)
	}
	p := m.place(top)
	shown := p < m.count() && m.at(p) == top
	m.top = p
	if !shown {
		m.row = 0
	}
	m.hits.valid = false
	m.clamp()
	if m.cursoring {
		m.snapCursor()
	}
}

// compose sets the lines shown from the mask and what the filter picked.
func (m *Model) compose() {
	switch {
	case m.mask == nil:
		m.vis = m.pvis
	case m.pvis == nil:
		// Rows that went since the mask was set are not shown.
		k, _ := slices.BinarySearch(m.mask, int32(m.rows.Len()))
		m.vis = m.mask[:k]
	default:
		m.vis = intersect(m.mask, m.pvis)
	}
}

// setPicked takes the lines the projection picked.
func (m *Model) setPicked(pvis []int32) {
	m.pvis = pvis
	m.compose()
}

// intersect returns the values that both ascending lists hold, never nil.
func intersect(a, b []int32) []int32 {
	out := make([]int32, 0, min(len(a), len(b)))
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}

// hiddenByMask reports whether the mask hides row i.
func (m Model) hiddenByMask(i int) bool {
	if m.mask == nil {
		return false
	}
	_, ok := slices.BinarySearch(m.mask, int32(i))
	return !ok
}

// stand returns the row that stands in for row i: its anchor if the mask
// hides it, or else i.
func (m Model) stand(i int) int {
	if m.maskAnchor != nil && m.hiddenByMask(i) {
		return m.maskAnchor(i)
	}
	return i
}

// place returns the position of row i among the lines shown, or of the row
// that stands in for it.
func (m Model) place(i int) int { return m.posOf(m.stand(i)) }

// searchVis returns the lines a search runs over: all that the filter shows
// when the host can reveal a hidden row, or else the lines shown.
func (m Model) searchVis() []int32 {
	if m.mask != nil && m.reveal != nil {
		return m.pvis
	}
	return m.vis
}

// revealRow opens the fold of row i through the host's hook if the mask hides
// it, and reports whether the pager may land on it.
func (m *Model) revealRow(i int) bool {
	if !m.hiddenByMask(i) || m.reveal == nil {
		return true
	}
	vis, changed := m.reveal(i)
	if !changed {
		return false
	}
	top := m.topLine()
	if vis == nil {
		m.mask = nil
	} else {
		m.mask = vis
	}
	m.applyMask(top)
	return !m.hiddenByMask(i)
}

package keyhelp

import (
	"slices"

	"github.com/sahilm/fuzzy"
)

// refilter lists the rows that match the query and the captured key, in
// the order of their layers, and scrolls to the top.
func (m *Model) refilter() {
	in := make([]int, 0, len(m.rows))
	for i, r := range m.rows {
		if m.key == "" || slices.Contains(r.Binding.Keys(), m.key) {
			in = append(in, i)
		}
	}
	if q := m.input.Value(); q != "" {
		// The rows keep their order, so each layer stays together.
		hay := make([]string, len(in))
		for i, r := range in {
			hay[i] = m.hay[r]
		}
		found := fuzzy.FindNoSort(q, hay)
		out := make([]int, len(found))
		for i, f := range found {
			out[i] = in[f.Index]
		}
		in = out
	}
	m.shown = in
	m.vp.GotoTop()
	m.relist()
}

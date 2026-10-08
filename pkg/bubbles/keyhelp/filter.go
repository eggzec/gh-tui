package keyhelp

import (
	"slices"
	"strings"

	"github.com/sahilm/fuzzy"
)

// configPath starts the path of an action in the config, which a query may
// filter by.
const configPath = "keys."

// refilter lists the rows that match the query and the captured key, in
// the order of their layers, and scrolls to the top.
func (m *Model) refilter() {
	in := make([]int, 0, len(m.rows))
	for i, r := range m.rows {
		if m.key == "" || slices.Contains(r.Binding.Keys(), m.key) {
			in = append(in, i)
		}
	}
	q := m.input.Value()
	if len(q) >= len(configPath) && strings.EqualFold(q[:len(configPath)], configPath) {
		// A config path, such as keys.pulls.merge, finds the rows whose
		// action it names, whole or in part.
		q = strings.ToLower(q)
		in = slices.DeleteFunc(in, func(i int) bool {
			return !slices.ContainsFunc(m.actions[i], func(a string) bool {
				return strings.Contains(configPath+strings.ToLower(a), q)
			})
		})
	} else if q != "" {
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
	m.list()
}

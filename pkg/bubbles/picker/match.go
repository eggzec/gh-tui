package picker

import (
	"cmp"
	"slices"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// result is an item as the picker lists it.
type result struct {
	Item
	// title and detail are the item's, on one line and without escape
	// sequences, so they can't break the layout.
	title, detail string
	// matches holds the byte offsets in title of the runes that match the
	// query, in order.
	matches []int
}

func newResult(it Item) result {
	return result{Item: it, title: clean(it.Title), detail: clean(it.Detail)}
}

// results returns the items as the picker lists them.
func results(items []Item) []result {
	out := make([]result, len(items))
	for i := range items {
		out[i] = newResult(items[i])
	}
	return out
}

// row is a line of the list: a group header, or an item.
type row struct {
	header string
	item   int
}

// typedHeader heads the row of the typed item.
const typedHeader = "Typed"

// titles lets fuzzy match the titles of results.
type titles []result

func (t titles) String(i int) string { return t[i].title }
func (t titles) Len() int            { return len(t) }

// filter returns the fixed items in q's scope that match q's text, best
// match first, or all of them in order for an empty text.
func (m Model) filter(q Query) []result {
	in := make([]result, 0, len(m.pool))
	for i := range m.pool {
		if q.Scope == "" || m.pool[i].Kind == q.Scope {
			in = append(in, m.pool[i])
		}
	}
	if q.Text == "" {
		return in
	}
	found := fuzzy.FindFrom(q.Text, titles(in))
	out := make([]result, len(found))
	for i, f := range found {
		out[i] = in[f.Index]
		out[i].matches = f.MatchedIndexes
	}
	return out
}

// searched returns the items a search found, in the order it found them,
// with the matches of text marked where the titles have them.
func searched(items []Item, text string) []result {
	out := results(items)
	if text != "" {
		for _, f := range fuzzy.FindFromNoSort(text, titles(out)) {
			out[f.Index].matches = f.MatchedIndexes
		}
	}
	return out
}

// show lists rs, grouped by kind, and selects the first.
func (m *Model) show(rs []result) {
	group(rs)
	m.listed = rs
	m.sel, m.top = 0, 0
	m.rebuild()
}

// rebuild lists what was found and the typed item, if one is due, keeping
// the selection where it can stay.
func (m *Model) rebuild() {
	rs := m.listed
	m.typedAt = false
	if it, ok := m.typedItem(); ok {
		// A new slice, since copies of the model share the old one.
		rs = append(slices.Clip(slices.Clone(rs)), newResult(it))
		m.typedAt = true
	}
	m.results = rs
	// Copies of the model share the old slices, so these are new ones.
	m.rows, m.itemRow = make([]row, 0, len(rs)+4), make([]int, 0, len(rs))
	for i, r := range rs {
		switch {
		case m.typedAt && i == len(rs)-1:
			// A header of its own, so the row doesn't read as part of the
			// last group.
			m.rows = append(m.rows, row{header: typedHeader, item: -1})
		case m.headers && r.Kind != "" && (i == 0 || rs[i-1].Kind != r.Kind):
			m.rows = append(m.rows, row{header: r.Kind, item: -1})
		}
		m.itemRow = append(m.itemRow, len(m.rows))
		m.rows = append(m.rows, row{item: i})
	}
	m.lines = make([]string, len(m.rows))
	m.scroll()
}

// typedItem returns the item the user can choose for what they typed, if
// the picker offers one now. It does so in either mode, so the text stays
// choosable after esc.
func (m Model) typedItem() (Item, bool) {
	text := m.input.Value()
	if m.typed == nil || text == "" || m.loading || m.err != nil {
		return Item{}, false
	}
	// Only what is listed counts, which is what the scope shows: the same
	// text may be offered again in another scope, which is intended.
	want := strings.TrimSpace(text)
	for _, r := range m.listed {
		if strings.EqualFold(strings.TrimSpace(r.Title), want) {
			return Item{}, false
		}
		if v, ok := r.Value.(string); ok && strings.EqualFold(strings.TrimSpace(v), want) {
			return Item{}, false
		}
	}
	return m.typed(text)
}

// retype lists the typed item again if it should now appear or go.
func (m *Model) retype() {
	_, want := m.typedItem()
	if want != m.typedAt {
		m.rebuild()
	}
}

// group sorts rs by kind, the kinds in the order they first appear, and
// keeps the order within each kind.
func group(rs []result) {
	order := make(map[string]int)
	for _, r := range rs {
		if _, ok := order[r.Kind]; !ok {
			order[r.Kind] = len(order)
		}
	}
	if len(order) < 2 {
		return
	}
	slices.SortStableFunc(rs, func(a, b result) int {
		return cmp.Compare(order[a.Kind], order[b.Kind])
	})
}

// clean puts text on one line without escape sequences.
func clean(s string) string {
	return strings.Join(strings.Fields(termtext.OneLine(s)), " ")
}

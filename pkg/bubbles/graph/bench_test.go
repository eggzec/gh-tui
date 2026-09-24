package graph

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchGraph returns a graph with a 40-row window over 10,000 commits loaded
// in chunks of 100, with the cursor in the middle.
func benchGraph(b *testing.B) Model {
	b.Helper()
	m := load(b, newSource(history(10_000), 100), WithSize(100, 40))
	for m.Len() < 10_000 {
		m, _ = keys(b, m, "G")
	}
	m.sel = 5_000
	m.scroll()
	return m
}

func BenchmarkView(b *testing.B) {
	m := benchGraph(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := benchGraph(b)
	// Box the keys once so the benchmark measures the graph, not the boxing.
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a full window down and back so the window scrolls.
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		m, _ = m.Update(msg)
	}
}

// BenchmarkLayout measures laying out a chunk of 100 commits.
func BenchmarkLayout(b *testing.B) {
	commits := history(10_000)
	l := newLayout(DefaultMaxLanes)
	seen := map[string]struct{}{}
	isSeen := func(id string) bool {
		_, ok := seen[id]
		return ok
	}
	cells := make([]cell, 0, 100*4)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i == len(commits) {
			l, i = newLayout(DefaultMaxLanes), 0
			clear(seen)
		}
		cells = cells[:0]
		for _, c := range commits[i : i+100] {
			seen[c.ID] = struct{}{}
			cells = l.add(c, isSeen, cells)
		}
		i += 100
	}
}

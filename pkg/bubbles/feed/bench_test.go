package feed

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchFeed returns a feed with a 40-row window over 10,000 items in chunks
// of 100, scrolled to the middle.
func benchFeed(b *testing.B) Model[item] {
	b.Helper()
	src := newSource(10_000, 100)
	m := load(b, src, WithSize(100, 40))
	for m.Len() < 10_000 {
		m = keys(b, m, "end")
	}
	m.sel = 5_000
	m.scroll()
	return m
}

func BenchmarkView(b *testing.B) {
	m := benchFeed(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := benchFeed(b)
	// Box the keys once so the benchmark measures the feed, not the boxing.
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
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		_ = cmd
	}
}

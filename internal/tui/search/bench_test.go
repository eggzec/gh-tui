package search

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchSection returns the page at 140x38 with the repositories of a
// query in view, the results focused.
func benchSection(b *testing.B) *Section {
	b.Helper()
	s := newSection(b, newFake(), 140, 38)
	typeText(b, s, "tea")
	press(b, s, "down")
	return s
}

func BenchmarkView(b *testing.B) {
	s := benchSection(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = s.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	s := benchSection(b)
	// Box the keys once so the benchmark measures the page.
	down, up := tea.Msg(keyPress("down")), tea.Msg(keyPress("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk the first page down and back so the list scrolls.
		msg := down
		if i/15%2 == 1 {
			msg = up
		}
		i++
		if cmd := s.Update(msg); cmd != nil {
			run(b, s, cmd)
		}
	}
}

func BenchmarkType(b *testing.B) {
	s := newSection(b, newFake(), 140, 38)
	keys := []tea.Msg{keyPress("t"), keyPress("e"), keyPress("a"), keyPress("backspace"), keyPress("backspace"), keyPress("backspace")}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if cmd := s.Update(keys[i%len(keys)]); cmd != nil {
			run(b, s, cmd)
		}
		i++
	}
}

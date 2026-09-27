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

// BenchmarkType types faster than the other kinds wait for: the wait of
// each key ends after the next key, when a newer edit makes it moot.
func BenchmarkType(b *testing.B) {
	s := newSection(b, newFake(), 140, 38)
	keys := []tea.Msg{keyPress("t"), keyPress("e"), keyPress("a"), keyPress("backspace"), keyPress("backspace"), keyPress("backspace")}
	others := func(msg tea.Msg) bool { _, ok := msg.(othersMsg); return ok }
	// The waits of the last key, and those of the key before, which end
	// now; the two swap each key.
	waits, ended := make([]tea.Msg, 0, 1), make([]tea.Msg, 0, 1)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		waits, ended = ended[:0], waits
		if cmd := s.Update(keys[i%len(keys)]); cmd != nil {
			_, waits = drive(b, s, cmd, others, waits)
		}
		for _, w := range ended {
			run(b, s, s.Update(w))
		}
		i++
	}
}

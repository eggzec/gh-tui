package issues

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchSection returns a section at 120×40 over 300 issues, all loaded,
// with the selection in the middle.
func benchSection(b *testing.B) *Section {
	b.Helper()
	s := started(b, newFakeService(sampleIssues(300)), 120, 40)
	press(b, s, "G")
	for range 100 {
		press(b, s, "up")
	}
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
	down, up := tea.Msg(keyMsg("down")), tea.Msg(keyMsg("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a window down and back, so the list scrolls.
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		if cmd := s.Update(msg); cmd != nil {
			run(b, s, cmd)
		}
	}
}

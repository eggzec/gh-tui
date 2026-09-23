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

func BenchmarkViewDetail(b *testing.B) {
	svc := newFakeService(sampleIssues(30))
	svc.addComments(999, sampleComments(90)...)
	s := started(b, svc, 120, 40)
	press(b, s, "down", "enter")
	press(b, s, "d", "d")
	b.ReportAllocs()
	for b.Loop() {
		_ = s.View()
	}
}

// BenchmarkUpdateCompose types into the comment prompt, with the thread
// above it.
func BenchmarkUpdateCompose(b *testing.B) {
	svc := newFakeService(sampleIssues(30))
	svc.addComments(999, sampleComments(90)...)
	s := started(b, svc, 120, 40)
	press(b, s, "down", "enter", "c")
	typeText(b, s, "I can reproduce this on main with an empty config file.")
	a, bksp := tea.Msg(keyMsg("a")), tea.Msg(tea.KeyPressMsg{Code: tea.KeyBackspace})
	b.ReportAllocs()
	for b.Loop() {
		s.Update(a)
		s.Update(bksp)
	}
}

func BenchmarkViewCompose(b *testing.B) {
	svc := newFakeService(sampleIssues(30))
	svc.addComments(999, sampleComments(90)...)
	s := started(b, svc, 120, 40)
	press(b, s, "down", "enter", "c")
	typeText(b, s, "I can reproduce this on main with an empty config file.")
	b.ReportAllocs()
	for b.Loop() {
		_ = s.View()
	}
}

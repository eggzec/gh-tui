package dashboard

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchSection returns the dashboard at 140x38 over 1,000 repositories of
// the viewer, with the list scrolled into the second page.
func benchSection(b *testing.B) *Section {
	b.Helper()
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 1000)
	return newSection(b, svc, &fakeInbox{threads: inboxThreads()}, 140, 38)
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
	// Box the keys once so the benchmark measures the dashboard.
	down, up := tea.Msg(keyPress("down")), tea.Msg(keyPress("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a window down and back so the list scrolls.
		msg := down
		if i/30%2 == 1 {
			msg = up
		}
		i++
		if cmd := s.Update(msg); cmd != nil {
			run(b, s, cmd)
		}
	}
}

func BenchmarkFilter(b *testing.B) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 1000)
	s := newSection(b, svc, nil, 140, 38)
	press(b, s, "f")
	keys := []tea.Msg{keyPress("r"), keyPress("7"), tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyBackspace}}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if cmd := s.Update(keys[i%len(keys)]); cmd != nil {
			run(b, s, cmd)
		}
		i++
	}
}

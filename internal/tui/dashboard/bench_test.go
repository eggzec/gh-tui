package dashboard

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
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

func BenchmarkViewZoomed(b *testing.B) {
	s := benchSection(b)
	press(b, s, "z")
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

// BenchmarkFilter applies filters in turn over 1,000 repositories of the
// viewer, all read, and lists what they keep.
func BenchmarkFilter(b *testing.B) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 1000)
	s := newSection(b, svc, nil, 140, 38)
	queries := []string{"is:private language:go sort:stars-desc", "r7", "archived:false sort:name-asc"}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		run(b, s, s.ApplyFilter(filterform.AppliedMsg{Query: queries[i%len(queries)]}))
		i++
	}
}

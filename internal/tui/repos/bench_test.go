package repos

import (
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// benchSection returns a section at 120 by 40 over 1,000 repos, scrolled
// into the middle of the loaded ones.
func benchSection(b *testing.B) *Section {
	b.Helper()
	sample := sampleRepos()
	repos := make([]core.Repo, 1_000)
	for i := range repos {
		repos[i] = sample[i%len(sample)]
		repos[i].Ref.Name += "-" + strconv.Itoa(i)
	}
	s := newSection(b, newFake(repos...), 120, 40)
	for range 50 {
		keys(s, "down")
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
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a window down and back so the list scrolls.
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		run(s, s.Update(msg))
	}
}

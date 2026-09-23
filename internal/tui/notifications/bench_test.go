package notifications

import (
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// benchSection returns a section at 120x40 over 3,000 threads in pages of
// 50, showing read and unread ones.
func benchSection(b *testing.B) *Section {
	b.Helper()
	base := inbox()
	threads := make([]core.Notification, 3000)
	for i := range threads {
		n := base[i%len(base)]
		n.ID = strconv.Itoa(i)
		n.UpdatedAt = now.Add(-time.Duration(i) * time.Minute)
		threads[i] = n
	}
	svc := newFake(threads...)
	svc.size = 50
	s := newSection(b, svc, 120, 40)
	press(b, s, "f")
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
	// Box the keys once so the benchmark measures the section.
	down, up := tea.Msg(keyPress("down")), tea.Msg(keyPress("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a full window down and back so the list scrolls.
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

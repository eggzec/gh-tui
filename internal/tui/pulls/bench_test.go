package pulls

import (
	"testing"
	"time"
)

func benchSection(b *testing.B, opts ...Option) *host {
	b.Helper()
	svc := newFakeService()
	svc.pulls = manyPulls(300)
	return started(b, svc, 120, 40, opts...)
}

func BenchmarkView(b *testing.B) {
	b.Run("list", func(b *testing.B) {
		s := benchSection(b)
		b.ReportAllocs()
		for b.Loop() {
			_ = s.View()
		}
	})
	b.Run("list at 40 columns", func(b *testing.B) {
		s := benchSection(b)
		s.SetSize(40, 20)
		drain(b, s, s.Update(nil))
		b.ReportAllocs()
		for b.Loop() {
			_ = s.View()
		}
	})
	b.Run("modal", func(b *testing.B) {
		s := benchSection(b)
		press(b, s, "enter")
		m := s.modal()
		b.ReportAllocs()
		for b.Loop() {
			_ = m.View()
		}
	})
}

func BenchmarkUpdate(b *testing.B) {
	b.Run("move", func(b *testing.B) {
		s := benchSection(b)
		down, up := keyMsg("down"), keyMsg("up")
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			// Stay within the first page, so no fetch runs.
			k := down
			if i%40 >= 20 {
				k = up
			}
			i++
			_ = s.Update(k)
		}
	})
	b.Run("move with prefetch", func(b *testing.B) {
		// Each move starts the hover delay, which the loop doesn't run.
		s := benchSection(b, WithPrefetch(5, time.Nanosecond))
		down, up := keyMsg("down"), keyMsg("up")
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			k := down
			if i%40 >= 20 {
				k = up
			}
			i++
			_ = s.Update(k)
		}
	})
	b.Run("scroll modal", func(b *testing.B) {
		s := benchSection(b)
		press(b, s, "enter")
		down, up := keyMsg("down"), keyMsg("up")
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			k := down
			if i%20 >= 10 {
				k = up
			}
			i++
			_ = s.Update(k)
		}
	})
}

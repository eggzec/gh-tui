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

// benchPanes returns the section with the modal of its first pull request
// open, in a terminal of 160 by 50 cells.
func benchPanes(b *testing.B) (*host, *panesModal) {
	b.Helper()
	svc := newFakeService()
	svc.pulls = manyPulls(300)
	h := started(b, svc, 160, 50)
	press(b, h, "enter")
	m := h.modal()
	if m == nil || !m.loaded {
		b.Fatalf("enter opened %v, want a loaded modal", m)
	}
	return h, m
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
		_, m := benchPanes(b)
		b.ReportAllocs()
		for b.Loop() {
			_ = m.View()
		}
	})
	b.Run("modal with an overlay", func(b *testing.B) {
		_, m := benchPanes(b)
		if _, ok := m.OpenHelp(m.KeyLayers(), "Help"); !ok || m.top() == nil {
			b.Fatal("OpenHelp opened no overlay")
		}
		b.ReportAllocs()
		for b.Loop() {
			_ = m.View()
		}
	})
	b.Run("modal zoomed", func(b *testing.B) {
		h, m := benchPanes(b)
		press(b, h, "z")
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
		s := benchSection(b, readingAhead(4, time.Nanosecond, true))
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
	b.Run("focus panes", func(b *testing.B) {
		h, _ := benchPanes(b)
		next := keyMsg("tab")
		b.ReportAllocs()
		for b.Loop() {
			_ = h.Update(next)
		}
	})
}

package pulls

import (
	"fmt"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
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
	b.Run("files tab", func(b *testing.B) {
		h, m := filed(b, newFakeService(), 120, 40)
		press(b, h, "[")
		b.ReportAllocs()
		for b.Loop() {
			_ = m.View()
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
	b.Run("modal asking to merge", func(b *testing.B) {
		s := benchSection(b)
		press(b, s, "enter")
		press(b, s, "M")
		m := s.modal()
		if m.ask == nil {
			b.Fatal("merge asked nothing")
		}
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

// BenchmarkFilesGrow reads the 3000 files GitHub lists at most, a page at a
// time, and so grows the tree and the diff.
func BenchmarkFilesGrow(b *testing.B) {
	svc := newFakeService()
	svc.filePage = 100
	for i := range core.MaxPullFiles {
		svc.changed = append(svc.changed, core.CommitFile{
			Path: fmt.Sprintf("pkg%d/sub%d/file%02d.go", i/1000, i/100%10, i%100), Status: core.FileModified, Additions: 1,
		})
	}
	b.ReportAllocs()
	for b.Loop() {
		h, m := filed(b, svc, 120, 40)
		press(b, h, "[")
		for m.files.diff.Files() < core.MaxPullFiles {
			press(b, h, "G")
		}
	}
}

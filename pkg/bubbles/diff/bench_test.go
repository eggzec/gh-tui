package diff

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchView returns a view of 3000 files in one page, with a 40-row window
// scrolled into the middle of a file.
func benchView(b *testing.B) Model {
	b.Helper()
	files := make([]File, 3000)
	for i := range files {
		files[i] = goFile(fmt.Sprintf("pkg/dir%d/file%d.go", i/50, i), 8)
	}
	src := &source{size: len(files), files: files}
	m := load(b, src, WithSize(120, 40))
	row, _ := m.layout.FileRow(1500)
	m.cursor, m.top = row+10, row+8
	return run(b, m, m.sync())
}

func BenchmarkView(b *testing.B) {
	m := benchView(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := benchView(b)
	// Box the keys once so the benchmark measures the view, not the boxing.
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a full window down and back so the window scrolls.
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		m, _ = m.Update(msg)
	}
}

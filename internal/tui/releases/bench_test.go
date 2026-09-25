package releases

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func BenchmarkView(b *testing.B) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"190 columns", 148, 38}, {"80 columns", 60, 18}} {
		b.Run(size.name, func(b *testing.B) {
			m := newModal(&fakeService{cached: true}, size.w, size.h)
			_ = m.Init()
			b.ReportAllocs()
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}

// BenchmarkUpdate measures scrolling through the notes and the files.
func BenchmarkUpdate(b *testing.B) {
	m := newModal(&fakeService{cached: true}, 148, 12)
	_ = m.Init()
	down, up := tea.KeyPressMsg{Code: 'j', Text: "j"}, tea.KeyPressMsg{Code: 'k', Text: "k"}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := down
		if i/10%2 == 1 {
			k = up
		}
		i++
		_ = m.Update(k)
	}
}

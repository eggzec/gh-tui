package jobview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func BenchmarkView(b *testing.B) {
	m := newView(b, newFake(), 120, 36)
	run(m, m.Show(failed(), false, Hints{}))
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkUpdate measures moving through the log.
func BenchmarkUpdate(b *testing.B) {
	m := newView(b, newFake(), 120, 36)
	run(m, m.Show(failed(), false, Hints{}))
	m.Focus()
	down, up := tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyUp}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := down
		if i/3%2 == 1 {
			k = up
		}
		i++
		*m, _ = m.Update(k)
	}
}

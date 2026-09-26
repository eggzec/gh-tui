package cmdline

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func BenchmarkView(b *testing.B) {
	m := opened(b, longLine, WithSize(120, MaxHeight))
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	// Type a character and delete it, so the line stays the same size.
	b.Run("type", func(b *testing.B) {
		m := opened(b, longLine, WithSize(120, MaxHeight))
		a := tea.Msg(runeKey("a"))
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(a)
			m, _ = m.Update(bksp)
		}
	})
	// A message that changes nothing, such as another bubble's tick.
	b.Run("ignored", func(b *testing.B) {
		m := opened(b, longLine, WithSize(120, MaxHeight))
		msg := tea.Msg(struct{}{})
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(msg)
		}
	})
	b.Run("blurred", func(b *testing.B) {
		m := New(WithValue(longLine), WithSize(120, MaxHeight))
		a := tea.Msg(runeKey("a"))
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(a)
		}
	})
}

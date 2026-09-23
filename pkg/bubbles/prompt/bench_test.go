package prompt

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func benchPrompt(b *testing.B, mode Mode) Model {
	b.Helper()
	m := focused(b, WithMode(mode), WithTitle("Comment on #999"), WithSize(120, 10))
	return typeText(b, m, longComment)
}

func BenchmarkView(b *testing.B) {
	for _, mode := range []struct {
		name string
		mode Mode
	}{{"multi-line", MultiLine}, {"single-line", SingleLine}} {
		b.Run(mode.name, func(b *testing.B) {
			m := benchPrompt(b, mode.mode)
			b.ReportAllocs()
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}

func BenchmarkUpdate(b *testing.B) {
	for _, mode := range []struct {
		name string
		mode Mode
	}{{"multi-line", MultiLine}, {"single-line", SingleLine}} {
		// Type a character and delete it, so the value stays the same size.
		b.Run(mode.name+"/type", func(b *testing.B) {
			m := benchPrompt(b, mode.mode)
			a := tea.Msg(runeKey("a"))
			b.ReportAllocs()
			for b.Loop() {
				m, _ = m.Update(a)
				m, _ = m.Update(bksp)
			}
		})
	}
	b.Run("blurred", func(b *testing.B) {
		m := benchPrompt(b, MultiLine)
		m.Blur()
		a := tea.Msg(runeKey("a"))
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(a)
		}
	})
}

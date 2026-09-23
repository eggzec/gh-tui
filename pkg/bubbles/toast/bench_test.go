package toast

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func benchModel() Model {
	m := New(WithSize(80, 24), WithDuration(0), WithErrorDuration(0))
	m.Push(Info, "Refreshing pull requests")
	m.Push(Success, "Merged #42")
	m.Push(Error, "Could not label #7: resource not accessible, rolled back")
	return m
}

func BenchmarkView(b *testing.B) {
	m := benchModel()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	b.Run("ignored key", func(b *testing.B) {
		m := benchModel()
		msg := tea.KeyPressMsg{Code: 'j', Text: "j"}
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(msg)
		}
	})
	// A push and its expiry, each of which renders the stack again.
	b.Run("push and expire", func(b *testing.B) {
		m := benchModel()
		b.ReportAllocs()
		for b.Loop() {
			m.Push(Warning, "Rate limit low")
			m, _ = m.Update(ExpireMsg{m.ID(), m.seq})
		}
	})
}

package filterform

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchSpec returns a spec of ten fields of every kind, set to values.
func benchSpec() (spec Spec, query string) {
	s := staticSpec()
	for i := range 4 {
		s.Fields = append(s.Fields, Field{
			Key: fmt.Sprintf("extra%d", i), Label: fmt.Sprintf("Extra %d", i), Kind: Multi,
			Qualifier: fmt.Sprintf("extra%d", i), Options: labels,
			Default: ListValue("bug", "good first issue"),
		})
	}
	return s, prDefaults + " -is:draft fix crash repo:cli/cli"
}

func benchForm(b *testing.B) Model {
	b.Helper()
	s, q := benchSpec()
	m := New(s, WithQuery(q), WithSize(100, 24))
	m.Focus()
	return m
}

func BenchmarkView(b *testing.B) {
	m := benchForm(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkViewSort renders the Sort tab.
func BenchmarkViewSort(b *testing.B) {
	m := benchForm(b)
	m.SetTab(SortTab)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	b.Run("choose", func(b *testing.B) {
		m := benchForm(b)
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(right)
		}
	})
	b.Run("move", func(b *testing.B) {
		m := benchForm(b)
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(down)
			m, _ = m.Update(up)
		}
	})
	// Typing on the query line parses the query on every key.
	b.Run("type query", func(b *testing.B) {
		m := benchForm(b)
		m, _ = press(b, m, keyBigG, keyA)
		a := tea.Msg(tea.KeyPressMsg{Code: 'a', Text: "a"})
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(a)
			m, _ = m.Update(bksp)
		}
	})
	b.Run("switch tab", func(b *testing.B) {
		m := benchForm(b)
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(nextTab)
		}
	})
	b.Run("choose sort", func(b *testing.B) {
		m := benchForm(b)
		m.SetTab(SortTab)
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(right)
		}
	})
	b.Run("toggle in picker", func(b *testing.B) {
		m := benchForm(b)
		m, _ = press(b, m, down, down, down, space, down, down)
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(space)
		}
	})
}

func BenchmarkQuery(b *testing.B) {
	m := benchForm(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.Query()
	}
}

func BenchmarkSetQuery(b *testing.B) {
	m := benchForm(b)
	_, q := benchSpec()
	b.ReportAllocs()
	for b.Loop() {
		m.SetQuery(q)
	}
}

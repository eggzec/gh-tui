package picker

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchItems returns n results of three kinds.
func benchItems(n int) []Item {
	kinds := []string{kindRepos, kindIssues, kindPulls}
	items := make([]Item, n)
	for i := range items {
		items[i] = Item{
			Kind:   kinds[i%len(kinds)],
			Title:  fmt.Sprintf("Fix the crash when the config file %d is empty", i),
			Detail: fmt.Sprintf("octo-org/hello#%d", i),
		}
	}
	return items
}

func benchSearch(items []Item) Search {
	return func(context.Context, Query) ([]Item, error) { return items, nil }
}

func BenchmarkView(b *testing.B) {
	m := open(b, benchSearch(benchItems(100)), WithSize(80, 30))
	m = typeText(b, m, "crash")
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	items := benchItems(100)
	// Type a character and delete it: with the debounce on, each key only
	// schedules a search.
	b.Run("type", func(b *testing.B) {
		m := New(benchSearch(items), WithSize(80, 30))
		m.Focus()
		m, _ = run(b, m, m.Init())
		a := tea.Msg(tea.KeyPressMsg{Code: 'a', Text: "a"})
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(a)
			m, _ = m.Update(bksp)
		}
	})
	b.Run("results", func(b *testing.B) {
		m := open(b, benchSearch(items), WithSize(80, 30))
		m = typeText(b, m, "crash")
		msg := resultMsg{id: m.ID(), seq: m.seq, text: "crash", items: items}
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(msg)
		}
	})
	b.Run("move", func(b *testing.B) {
		m := open(b, benchSearch(items), WithSize(80, 30))
		m = typeText(b, m, "crash")
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(down)
			m, _ = m.Update(up)
		}
	})
	b.Run("filter", func(b *testing.B) {
		m := open(b, nil, WithItems(benchItems(1000)), WithSize(80, 30))
		m = typeText(b, m, "cras")
		h := tea.Msg(tea.KeyPressMsg{Code: 'h', Text: "h"})
		b.ReportAllocs()
		for b.Loop() {
			m, _ = m.Update(h)
			m, _ = m.Update(bksp)
		}
	})
}

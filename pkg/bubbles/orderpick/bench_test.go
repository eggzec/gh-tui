package orderpick

import (
	"fmt"
	"testing"
)

func benchItems(n int) []Item {
	items := make([]Item, n)
	for i := range items {
		items[i] = Item{Value: i, Title: fmt.Sprintf("Field %d", i), AscLabel: "low to high", DescLabel: "high to low"}
	}
	return items
}

func BenchmarkView(b *testing.B) {
	m := open(b, WithItems(benchItems(100)))
	m, _ = press(b, m, space, space, space, tab)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := open(b, WithItems(benchItems(100)))
	m, _ = press(b, m, space, space, space, tab)
	b.ReportAllocs()
	for b.Loop() {
		m, _ = m.Update(letter('J'))
		m, _ = m.Update(letter('K'))
	}
}

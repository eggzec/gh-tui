package statusbar

import "testing"

func BenchmarkView(b *testing.B) {
	left, right := testItems()
	m := New(WithItems(left, right), WithWidth(80))
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkLayout measures laying the bar out again, as a resize or new
// items do.
func BenchmarkLayout(b *testing.B) {
	left, right := testItems()
	m := New(WithItems(left, right))
	b.ReportAllocs()
	for b.Loop() {
		m.SetWidth(40)
		m.SetWidth(80)
	}
}

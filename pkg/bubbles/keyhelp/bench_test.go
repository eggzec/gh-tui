package keyhelp

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchLayers returns n layers of 20 bindings each, some sharing keys.
func benchLayers(n int) []Layer {
	out := make([]Layer, n)
	for i := range out {
		l := Layer{Source: fmt.Sprintf("layer %d", i)}
		for j := range 20 {
			l.Bindings = append(l.Bindings, bind(fmt.Sprintf("action %d of layer %d", j, i), fmt.Sprintf("ctrl+%c", 'a'+j), fmt.Sprintf("f%d", j)))
		}
		out[i] = l
	}
	return out
}

func BenchmarkView(b *testing.B) {
	m := New(WithLayers(benchLayers(5)), WithSize(80, 30))
	m.Focus()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := New(WithLayers(benchLayers(5)), WithSize(80, 30))
	m.Focus()
	// Type a letter and delete it, so each pair filters twice.
	keys := []tea.KeyPressMsg{{Code: 'a', Text: "a"}, {Code: tea.KeyBackspace}}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		m, _ = m.Update(keys[i%2])
		i++
	}
}

func BenchmarkAnalyze(b *testing.B) {
	layers := benchLayers(5)
	b.ReportAllocs()
	for b.Loop() {
		_ = Analyze(layers)
	}
}

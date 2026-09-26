package history

import "testing"

func BenchmarkView(b *testing.B) {
	for _, size := range []struct {
		name string
		w, h int
		keys []string
	}{
		{"140 columns", wideW, wideH, nil},
		{"140 columns zoomed", wideW, wideH, []string{"z"}},
		{"80 columns", narrowW, narrowH, nil},
	} {
		b.Run(size.name, func(b *testing.B) {
			m, h := newModal(b, newFake(), size.w, size.h)
			h.keys(append([]string{"j", "j", "enter"}, size.keys...)...)
			b.ReportAllocs()
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}

// BenchmarkUpdate measures moving the graph's cursor onto commits whose
// details are cached, as the reads ahead leave them.
func BenchmarkUpdate(b *testing.B) {
	f := newFake()
	for sha := range f.details {
		f.cached[sha] = true
	}
	m, _ := newModal(b, f, wideW, wideH)
	down, up := press("j"), press("k")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := down
		if i/20%2 == 1 {
			k = up
		}
		i++
		_ = m.Update(k)
	}
}

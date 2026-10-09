package refs

import "testing"

func BenchmarkView(b *testing.B) {
	for _, tt := range []struct {
		name string
		w, h int
		keys []string
	}{
		{"120 columns", wideW, wideH, nil},
		{"80 columns", narrowW, narrowH, nil},
		{"120 columns mentions", wideW, wideH, []string{"G", "enter"}},
		{"120 columns filtered", wideW, wideH, []string{"&", "b", "o", "b"}},
	} {
		b.Run(tt.name, func(b *testing.B) {
			s, h := newStep(b, newFake(), tt.w, tt.h)
			h.keys(tt.keys...)
			b.ReportAllocs()
			for b.Loop() {
				_ = s.View()
			}
		})
	}
}

// BenchmarkUpdate measures moving the cursor through the links.
func BenchmarkUpdate(b *testing.B) {
	s, _ := newStep(b, newFake(), wideW, wideH)
	down, up := press("j"), press("k")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := down
		if i/4%2 == 1 {
			k = up
		}
		i++
		_ = s.Update(k)
	}
}

// BenchmarkFilter measures a key typed in the filter over every link.
func BenchmarkFilter(b *testing.B) {
	s, h := newStep(b, newFake(), wideW, wideH)
	h.keys("&")
	typed, back := press("a"), press("backspace")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := typed
		if i%2 == 1 {
			k = back
		}
		i++
		_ = s.Update(k)
	}
}

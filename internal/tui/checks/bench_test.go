package checks

import "testing"

func BenchmarkView(b *testing.B) {
	for _, tt := range []struct {
		name string
		w, h int
		keys []string
	}{
		{"190 columns", wideW, wideH, nil},
		{"80 columns", narrowW, narrowH, nil},
		{"190 columns job", wideW, wideH, []string{"enter"}},
		{"190 columns detail", wideW, wideH, []string{"down", "down", "enter"}},
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

// BenchmarkUpdate measures moving the cursor through the checks.
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

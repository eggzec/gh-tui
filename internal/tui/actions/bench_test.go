package actions

import "testing"

func BenchmarkView(b *testing.B) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"190 columns", wideW, wideH}, {"80 columns", narrowW, narrowH}} {
		b.Run(size.name, func(b *testing.B) {
			m, h := newModal(b, newFake(), size.w, size.h)
			h.keys("tab", "tab")
			b.ReportAllocs()
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}

// BenchmarkUpdate measures moving the cursor of the jobs over jobs whose
// logs are in memory, as a second look at a run finds them.
func BenchmarkUpdate(b *testing.B) {
	f := newFake()
	for id := range int64(4) {
		f.logs[lintJob+id] = testLog()
		f.cachedLogs[lintJob+id] = true
	}
	m, _ := newModal(b, f, wideW, wideH)
	m.Update(press("tab"))
	down, up := press("j"), press("k")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := down
		if i/3%2 == 1 {
			k = up
		}
		i++
		_ = m.Update(k)
	}
}

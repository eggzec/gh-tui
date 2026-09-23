package pulls

import (
	"testing"
)

func benchSection(b *testing.B) *Section {
	b.Helper()
	svc := newFakeService()
	svc.pulls = manyPulls(300)
	return started(b, svc, 120, 40)
}

func BenchmarkView(b *testing.B) {
	s := benchSection(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = s.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	s := benchSection(b)
	down, up := keyMsg("down"), keyMsg("up")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Stay within the first page, so no fetch runs.
		k := down
		if i%40 >= 20 {
			k = up
		}
		i++
		_ = s.Update(k)
	}
}

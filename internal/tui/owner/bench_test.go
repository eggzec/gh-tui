package owner

import "testing"

func BenchmarkView(b *testing.B) {
	s := newSection(b, newFake(), "octocat", 120, 40)
	b.ReportAllocs()
	for b.Loop() {
		_ = s.View()
	}
}

// BenchmarkUpdate moves the cursor down and up the repositories, which
// renders the list and the profile again.
func BenchmarkUpdate(b *testing.B) {
	s := newSection(b, newFake(), "octocat", 120, 40)
	down, up := keyPress("down"), keyPress("k")
	b.ReportAllocs()
	for b.Loop() {
		s.Update(down)
		s.Update(up)
	}
}

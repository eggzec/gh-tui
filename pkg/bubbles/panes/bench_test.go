package panes

import "testing"

func BenchmarkView(b *testing.B) {
	st := DefaultStyles(true)
	l := Compute(Spec{Width: 160, Height: 50, Focus: Files, InvolvedRows: 6, DevelopmentRows: 5})
	c := make([]Content, len(l.Boxes))
	for i := range c {
		c[i] = Content{Info: []string{"some info"}, Body: "a line of content that is shown\nanother line of content"}
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = st.Draw(l, c)
	}
}

func BenchmarkCompute(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Compute(Spec{Width: 160, Height: 50, Focus: Files, InvolvedRows: 6, DevelopmentRows: 5})
	}
}

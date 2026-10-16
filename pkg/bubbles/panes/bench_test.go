package panes

import "testing"

func benchContent(Box) Content {
	return Content{Info: []string{"some info"}, Body: "a line of content that is shown\nanother line of content"}
}

func BenchmarkView(b *testing.B) {
	st := DefaultStyles(true)
	l := Compute(Spec{Width: 160, Height: 50, Focus: Files, InvolvedRows: 6, DevelopmentRows: 5})
	b.ReportAllocs()
	for b.Loop() {
		_ = st.Draw(l, benchContent)
	}
}

// BenchmarkViewZoomed draws a zoomed pane beside its preview, which carries
// the pane strip on the bottom border.
func BenchmarkViewZoomed(b *testing.B) {
	st := DefaultStyles(true)
	l := Compute(Spec{Width: 110, Height: 40, Focus: Files, InvolvedRows: 6, DevelopmentRows: 5})
	b.ReportAllocs()
	for b.Loop() {
		_ = st.Draw(l, benchContent)
	}
}

func BenchmarkCompute(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Compute(Spec{Width: 160, Height: 50, Focus: Files, InvolvedRows: 6, DevelopmentRows: 5})
	}
}

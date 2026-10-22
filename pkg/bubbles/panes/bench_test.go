package panes

import "testing"

func benchContent(Box) Content {
	return Content{
		TopRight: []string{"some info"},
		Body:     "a line of content that is shown\nanother line of content",
	}
}

func benchSpec(w int) Spec {
	return Spec{Width: w, Height: 50, Panes: testPanes, Focus: 2, Zoom: true}
}

func BenchmarkView(b *testing.B) {
	st := DefaultStyles(true)
	l := Compute(benchSpec(160))
	b.ReportAllocs()
	for b.Loop() {
		_ = st.Draw(l, benchContent)
	}
}

// BenchmarkViewSplit draws a zoomed pane beside its preview.
func BenchmarkViewSplit(b *testing.B) {
	st := DefaultStyles(true)
	l := Compute(benchSpec(110))
	b.ReportAllocs()
	for b.Loop() {
		_ = st.Draw(l, benchContent)
	}
}

func BenchmarkCompute(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Compute(benchSpec(160))
	}
}

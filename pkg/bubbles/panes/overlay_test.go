package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestOverlayRect(t *testing.T) {
	tests := []struct {
		name string
		o    Overlay
		w, h int
		want Rect
	}{
		{"viewer is inset on a wide area", Overlay{Kind: Viewer}, 120, 40, Rect{4, 2, 112, 36}},
		{"viewer fills a narrow area", Overlay{Kind: Viewer}, 119, 40, Rect{0, 0, 119, 40}},
		{"maximized viewer fills a wide area", Overlay{Kind: Viewer, Maximized: true}, 160, 50, Rect{0, 0, 160, 50}},
		{"picker is centred", Overlay{Kind: Picker, Items: 5}, 120, 40, Rect{37, 15, 46, 9}},
		{"picker height stops at 14", Overlay{Kind: Picker, Items: 30}, 120, 40, Rect{37, 13, 46, 14}},
		{"picker fills a narrow area", Overlay{Kind: Picker, Items: 5}, 100, 30, Rect{0, 0, 100, 30}},
		{"confirm has a minimum width", Overlay{Kind: Confirm, Text: 10}, 80, 24, Rect{20, 10, 40, 3}},
		{"confirm grows with its text", Overlay{Kind: Confirm, Text: 50}, 80, 24, Rect{11, 10, 58, 3}},
		{"confirm stops at the area", Overlay{Kind: Confirm, Text: 100}, 80, 24, Rect{1, 10, 78, 3}},
		{"confirm in a short area", Overlay{Kind: Confirm, Text: 10}, 80, 2, Rect{20, 0, 40, 2}},
		{"nothing in an empty area", Overlay{Kind: Picker, Items: 3}, 0, 0, Rect{}},
		{"tiny wide viewer", Overlay{Kind: Viewer}, 120, 3, Rect{4, 2, 112, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OverlayRect(tt.o, tt.w, tt.h); got != tt.want {
				t.Errorf("= %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestOverlayInside checks the overlays stay in the area at any size.
func TestOverlayInside(t *testing.T) {
	for _, w := range []int{0, 1, 5, 40, 80, 120, 200} {
		for _, h := range []int{0, 1, 3, 10, 40} {
			for _, o := range []Overlay{{Kind: Viewer}, {Kind: Picker, Items: 20}, {Kind: Confirm, Text: 300}} {
				r := OverlayRect(o, w, h)
				if r.X < 0 || r.Y < 0 || r.W < 0 || r.H < 0 || r.X+r.W > w || r.Y+r.H > h {
					t.Errorf("%dx%d %+v: %+v leaves the area", w, h, o, r)
				}
			}
		}
	}
}

// TestWideLayout draws the two columns: the seam between them, the titles
// with their info, and the fixed pane heights.
func TestWideLayout(t *testing.T) {
	st := DefaultStyles(true)
	l := Compute(Spec{Width: 120, Height: 20, Focus: Conversation, InvolvedRows: 5, DevelopmentRows: 4})
	c := make([]Content, len(l.Boxes))
	c[0] = Content{Info: []string{"34%"}, Body: "▌Summary\nAdds a small calc package"}
	c[1] = Content{Focus: true, Info: []string{"26 of 26 shown · filter: all", "26 of 26 shown"}, Body: "▌alice  commented"}
	c[2] = Content{Info: []string{"5 blockers"}, Body: "feat/calc → main"}
	c[5] = Content{Info: []string{"8 files · +67 −9 · 2 viewed", "8 · +67 −9"}, Body: "src/calc.go"}
	out := st.Draw(l, c)
	plain := ansi.Strip(out)
	for i, ln := range strings.Split(plain, "\n") {
		if i == 0 && !strings.Contains(ln, "╮╭─ 3 Merge") {
			t.Errorf("no seam on the top line: %q", ln)
		}
	}
	golden.RequireEqual(t, plain)
}

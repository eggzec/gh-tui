package panes

import "testing"

func TestOverlayRect(t *testing.T) {
	tests := []struct {
		name string
		o    Overlay
		w, h int
		want Rect
	}{
		{"viewer is inset on a wide area", Overlay{InsetMin: 120, Kind: Viewer}, 120, 40, Rect{4, 2, 112, 36}},
		{"viewer fills a narrow area", Overlay{InsetMin: 120, Kind: Viewer}, 119, 40, Rect{0, 0, 119, 40}},
		{"maximized viewer fills a wide area", Overlay{InsetMin: 120, Kind: Viewer, Maximized: true}, 160, 50, Rect{0, 0, 160, 50}},
		{"picker is centred", Overlay{InsetMin: 120, Kind: Picker, Items: 5}, 120, 40, Rect{37, 15, 46, 9}},
		{"picker height stops at 14", Overlay{InsetMin: 120, Kind: Picker, Items: 30}, 120, 40, Rect{37, 13, 46, 14}},
		{"picker fills a narrow area", Overlay{InsetMin: 120, Kind: Picker, Items: 5}, 100, 30, Rect{0, 0, 100, 30}},
		{"confirm has a minimum width", Overlay{InsetMin: 120, Kind: Confirm, Text: 10}, 80, 24, Rect{20, 10, 40, 3}},
		{"confirm grows with its text", Overlay{InsetMin: 120, Kind: Confirm, Text: 50}, 80, 24, Rect{11, 10, 58, 3}},
		{"confirm stops at the area", Overlay{InsetMin: 120, Kind: Confirm, Text: 100}, 80, 24, Rect{1, 10, 78, 3}},
		{"confirm in a short area", Overlay{InsetMin: 120, Kind: Confirm, Text: 10}, 80, 2, Rect{20, 0, 40, 2}},
		{"nothing in an empty area", Overlay{InsetMin: 120, Kind: Picker, Items: 3}, 0, 0, Rect{}},
		{"short wide viewer fills the area", Overlay{InsetMin: 120, Kind: Viewer}, 120, 3, Rect{0, 0, 120, 3}},
		{"viewer fills when the inset leaves 4 lines", Overlay{InsetMin: 120, Kind: Viewer}, 120, 8, Rect{0, 0, 120, 8}},
		{"viewer is inset with 5 lines left", Overlay{InsetMin: 120, Kind: Viewer}, 120, 9, Rect{4, 2, 112, 5}},
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
			for _, o := range []Overlay{{InsetMin: 120, Kind: Viewer}, {InsetMin: 120, Kind: Picker, Items: 20}, {InsetMin: 120, Kind: Confirm, Text: 300}} {
				r := OverlayRect(o, w, h)
				if r.X < 0 || r.Y < 0 || r.W < 0 || r.H < 0 || r.X+r.W > w || r.Y+r.H > h {
					t.Errorf("%dx%d %+v: %+v leaves the area", w, h, o, r)
				}
			}
		}
	}
}

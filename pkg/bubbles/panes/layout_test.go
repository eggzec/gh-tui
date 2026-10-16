package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func rects(l Layout) map[Pane]Rect {
	m := map[Pane]Rect{}
	for _, b := range l.Boxes {
		if b.Role == Preview {
			continue
		}
		m[b.Pane] = b.Rect
	}
	return m
}

func TestComputeHeights(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
		want map[Pane]Rect
	}{
		{
			"80x24 is zoomed on the focus",
			Spec{Width: 80, Height: 24, Focus: Conversation},
			map[Pane]Rect{Conversation: {0, 0, 80, 24}},
		},
		{
			"80x24 ignores the zoom flag, there is nothing else to show",
			Spec{Width: 80, Height: 24, Focus: Files, Zoom: true},
			map[Pane]Rect{Files: {0, 0, 80, 24}},
		},
		{
			"100x30 zoomed Merge is a list beside a preview",
			Spec{Width: 100, Height: 30, Focus: Merge},
			map[Pane]Rect{Merge: {0, 0, 46, 30}},
		},
		{
			"119 columns is still zoomed",
			Spec{Width: 119, Height: 30, Focus: Description},
			map[Pane]Rect{Description: {0, 0, 119, 30}},
		},
		{
			"120x40 wide",
			Spec{Width: 120, Height: 40, Focus: Description, InvolvedRows: 5, DevelopmentRows: 4},
			map[Pane]Rect{
				Description: {0, 0, 84, 13}, Conversation: {0, 13, 84, 27},
				Merge: {84, 0, 36, 11}, Involved: {84, 11, 36, 7},
				Development: {84, 18, 36, 6}, Files: {84, 24, 36, 16},
			},
		},
		{
			"160x50 wide",
			Spec{Width: 160, Height: 50, Focus: Files, InvolvedRows: 5, DevelopmentRows: 4},
			map[Pane]Rect{
				Description: {0, 0, 124, 16}, Conversation: {0, 16, 124, 34},
				Merge: {124, 0, 36, 11}, Involved: {124, 11, 36, 7},
				Development: {124, 18, 36, 6}, Files: {124, 24, 36, 26},
			},
		},
		{
			"120x20 shrinks the tallest first, Merge winning ties",
			Spec{Width: 120, Height: 20, Focus: Description, InvolvedRows: 5, DevelopmentRows: 4},
			map[Pane]Rect{
				Description: {0, 0, 84, 6}, Conversation: {0, 6, 84, 14},
				Merge: {84, 0, 36, 5}, Involved: {84, 5, 36, 5},
				Development: {84, 10, 36, 5}, Files: {84, 15, 36, 5},
			},
		},
		{
			"no default rows for Involved and Development keep their minimum heights",
			Spec{Width: 120, Height: 40, Focus: Description},
			map[Pane]Rect{
				Merge: {84, 0, 36, 11}, Involved: {84, 11, 36, 4},
				Development: {84, 15, 36, 3}, Files: {84, 18, 36, 22},
			},
		},
		{
			"unzoomed wide with a zoom flag fills with the focus",
			Spec{Width: 160, Height: 50, Focus: Development, Zoom: true},
			map[Pane]Rect{Development: {0, 0, 46, 50}},
		},
		{
			"a focus that is no pane counts as Description",
			Spec{Width: 80, Height: 24},
			map[Pane]Rect{Description: {0, 0, 80, 24}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rects(Compute(tt.spec))
			for p, want := range tt.want {
				if got[p] != want {
					t.Errorf("%s = %+v, want %+v", p.Title(), got[p], want)
				}
			}
		})
	}
}

func TestComputeSplit(t *testing.T) {
	for _, tt := range []struct {
		name  string
		w     int
		focus Pane
		boxes int
	}{
		{"99 columns has no preview", 99, Merge, 1},
		{"100 columns splits Merge", 100, Merge, 2},
		{"Involved splits", 110, Involved, 2},
		{"Development splits", 119, Development, 2},
		{"Files splits", 100, Files, 2},
		{"Description never splits", 119, Description, 1},
		{"Conversation never splits", 119, Conversation, 1},
		{"Files at 99 is the tree alone", 99, Files, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l := Compute(Spec{Width: tt.w, Height: 20, Focus: tt.focus})
			if len(l.Boxes) != tt.boxes {
				t.Fatalf("%d boxes, want %d", len(l.Boxes), tt.boxes)
			}
			if tt.boxes == 2 {
				list, _ := l.Box(tt.focus, List)
				prev, _ := l.Box(tt.focus, Preview)
				if list.Rect.W != ListWidth || prev.Rect.X != ListWidth || prev.Rect.W != tt.w-ListWidth {
					t.Errorf("list %+v, preview %+v", list.Rect, prev.Rect)
				}
			}
		})
	}
}

func TestInner(t *testing.T) {
	if got := (Rect{3, 4, 10, 6}).Inner(); got != (Rect{5, 5, 6, 4}) {
		t.Errorf("Inner = %+v", got)
	}
	if got := (Rect{0, 0, 3, 1}).Inner(); got.W != 0 || got.H != 0 {
		t.Errorf("tiny Inner = %+v", got)
	}
}

// TestFits checks, at awkward sizes, that every box lies in the area and
// that the drawing is exactly the size of the area.
func TestFits(t *testing.T) {
	st := DefaultStyles(true)
	for _, w := range []int{0, 1, 2, 5, 40, 80, 99, 100, 119, 120, 121, 160} {
		for _, h := range []int{0, 1, 2, 3, 4, 10, 24, 40} {
			for _, zoom := range []bool{false, true} {
				for p := Description; p <= Files; p++ {
					spec := Spec{Width: w, Height: h, Focus: p, Zoom: zoom, InvolvedRows: 6, DevelopmentRows: 9}
					l := Compute(spec)
					for _, b := range l.Boxes {
						r := b.Rect
						if r.X < 0 || r.Y < 0 || r.W < 0 || r.H < 0 || r.X+r.W > w || r.Y+r.H > h {
							t.Fatalf("%dx%d %v: %s box %+v leaves the area", w, h, spec, b.Pane.Title(), r)
						}
					}
					out := st.Draw(l, nil)
					lines := strings.Split(out, "\n")
					if len(lines) != h && (h != 0 || out != "") {
						t.Fatalf("%dx%d zoom=%v focus=%v: %d lines", w, h, zoom, p, len(lines))
					}
					for i, ln := range lines {
						if got := ansi.StringWidth(ln); got != w && h > 0 {
							t.Fatalf("%dx%d zoom=%v focus=%v: line %d is %d cells: %q", w, h, zoom, p, i, got, ansi.Strip(ln))
						}
					}
				}
			}
		}
	}
}

func TestPane(t *testing.T) {
	if got := Conversation.Title(); got != "2 Conversation" {
		t.Errorf("Title = %q", got)
	}
	if got := Development.Short(); got != "Dev" {
		t.Errorf("Short = %q", got)
	}
	if Pane(0).Valid() || Pane(7).Valid() || Pane(0).Title() != "" || Pane(9).Name() != "" || Pane(0).Short() != "" {
		t.Error("an out-of-range pane must be invalid and nameless")
	}
}

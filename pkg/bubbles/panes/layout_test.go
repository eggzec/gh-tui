package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// testPanes is a pane set that is nothing like a pull request's: three
// panes, of which only the second shows a list beside a preview.
var testPanes = []Def{
	{Name: "Inbox", Short: "In"},
	{Name: "Threads", Short: "Thr", Split: true},
	{Name: "Details", Short: "Det"},
}

func TestComputeZoom(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec Spec
		want []Box
	}{
		{
			"a whole pane fills the area",
			Spec{Width: 80, Height: 24, Panes: testPanes, Focus: 3, Zoom: true},
			[]Box{{Pane: 3, Rect: Rect{0, 0, 80, 24}}},
		},
		{
			"a Split pane below the split width is whole",
			Spec{Width: 99, Height: 30, Panes: testPanes, Focus: 2, Zoom: true},
			[]Box{{Pane: 2, Rect: Rect{0, 0, 99, 30}}},
		},
		{
			"a Split pane at the split width has a list and a preview",
			Spec{Width: 100, Height: 30, Panes: testPanes, Focus: 2, Zoom: true},
			[]Box{
				{Pane: 2, Role: List, Rect: Rect{0, 0, 46, 30}},
				{Pane: 2, Role: Preview, Rect: Rect{46, 0, 54, 30}},
			},
		},
		{
			"a pane that is not Split never splits",
			Spec{Width: 200, Height: 30, Panes: testPanes, Focus: 1, Zoom: true},
			[]Box{{Pane: 1, Rect: Rect{0, 0, 200, 30}}},
		},
		{
			"the split width and the list width can be set",
			Spec{Width: 70, Height: 10, Panes: testPanes, Focus: 2, Zoom: true, SplitMin: 60, ListWidth: 20},
			[]Box{
				{Pane: 2, Role: List, Rect: Rect{0, 0, 20, 10}},
				{Pane: 2, Role: Preview, Rect: Rect{20, 0, 50, 10}},
			},
		},
		{
			"a focus that is no pane counts as the first",
			Spec{Width: 80, Height: 24, Panes: testPanes, Focus: 7, Zoom: true},
			[]Box{{Pane: 1, Rect: Rect{0, 0, 80, 24}}},
		},
		{
			"without Zoom the caller arranges the boxes",
			Spec{Width: 160, Height: 24, Panes: testPanes, Focus: 2},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := Compute(tt.spec)
			if len(l.Boxes) != len(tt.want) {
				t.Fatalf("boxes = %+v, want %+v", l.Boxes, tt.want)
			}
			for i, b := range l.Boxes {
				if b != tt.want[i] {
					t.Errorf("box %d = %+v, want %+v", i, b, tt.want[i])
				}
			}
		})
	}
}

func TestComputeKeepsNineDefs(t *testing.T) {
	t.Parallel()
	l := Compute(Spec{Width: 10, Height: 5, Panes: make([]Def, 12), Focus: 12, Zoom: true})
	if len(l.Panes) != 9 || l.Focus != 1 {
		t.Errorf("%d panes, focus %d; want 9 and 1", len(l.Panes), l.Focus)
	}
}

func TestTitle(t *testing.T) {
	t.Parallel()
	l := Compute(Spec{Panes: testPanes})
	if got := l.Title(2); got != "2 Threads" {
		t.Errorf("Title(2) = %q", got)
	}
	if l.Title(0) != "" || l.Title(4) != "" {
		t.Error("a pane outside the layout has no title")
	}
}

func TestInner(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	st := DefaultStyles(true)
	for _, w := range []int{0, 1, 2, 5, 40, 80, 99, 100, 119, 120, 121, 160} {
		for _, h := range []int{0, 1, 2, 3, 4, 10, 24, 40} {
			for p := Pane(1); p <= 3; p++ {
				spec := Spec{Width: w, Height: h, Panes: testPanes, Focus: p, Zoom: true}
				l := Compute(spec)
				for _, b := range l.Boxes {
					r := b.Rect
					if r.X < 0 || r.Y < 0 || r.W < 0 || r.H < 0 || r.X+r.W > w || r.Y+r.H > h {
						t.Fatalf("%dx%d: %s box %+v leaves the area", w, h, l.Title(b.Pane), r)
					}
				}
				out := st.Draw(l, nil)
				lines := strings.Split(out, "\n")
				if len(lines) != h && (h != 0 || out != "") {
					t.Fatalf("%dx%d focus=%v: %d lines", w, h, p, len(lines))
				}
				for i, ln := range lines {
					if got := ansi.StringWidth(ln); got != w && h > 0 {
						t.Fatalf("%dx%d focus=%v: line %d is %d cells: %q", w, h, p, i, got, ansi.Strip(ln))
					}
				}
			}
		}
	}
}

func TestStripForms(t *testing.T) {
	t.Parallel()
	st := DefaultStyles(true)
	forms := st.StripForms(testPanes, 2)
	want := []string{
		"1 Inbox · 2 Threads · 3 Details",
		"1 In · 2 Thr · 3 Det",
		"1 In 2 Thr 3 Det",
		"1 [2] 3",
	}
	if len(forms) != len(want) {
		t.Fatalf("%d forms, want %d", len(forms), len(want))
	}
	for i, f := range forms {
		if got := ansi.Strip(f); got != want[i] {
			t.Errorf("form %d = %q, want %q", i, got, want[i])
		}
	}
	for i := 1; i < len(forms); i++ {
		if ansi.StringWidth(forms[i]) >= ansi.StringWidth(forms[i-1]) {
			t.Errorf("form %d is not shorter than form %d", i, i-1)
		}
	}
}

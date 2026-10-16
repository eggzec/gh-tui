package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func bottom(out string) string {
	lines := strings.Split(ansi.Strip(out), "\n")
	return lines[len(lines)-1]
}

func TestStripForms(t *testing.T) {
	st := DefaultStyles(true)
	forms := st.stripForms(Merge)
	got := make([]string, 0, len(forms))
	for _, f := range forms {
		if ansi.StringWidth(f.text) != f.width {
			t.Errorf("form %q has width %d", ansi.Strip(f.text), f.width)
		}
		got = append(got, ansi.Strip(f.text))
	}
	want := []string{
		"1 Description · 2 Conversation · 3 Merge · 4 Involved · 5 Development · 6 Files",
		"1 Desc · 2 Conv · 3 Merge · 4 Inv · 5 Dev · 6 Files",
		"1 Desc 2 Conv 3 Merge 4 Inv 5 Dev 6 Files",
		"1 2 [3] 4 5 6",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("forms:\n%s", strings.Join(got, "\n"))
	}
	// the current pane differs from the others by style
	cur, other := st.stripForms(Merge)[3].text, st.stripForms(Files)[3].text
	if cur == other {
		t.Error("the current pane must be styled differently")
	}
}

// TestStripWidths draws one zoomed pane at widths where each form is the
// longest that fits, and where none does.
func TestStripWidths(t *testing.T) {
	st := DefaultStyles(true)
	var b strings.Builder
	for _, w := range []int{90, 85, 84, 83, 57, 56, 55, 48, 47, 46, 20, 19, 18, 12} {
		l := Layout{Width: w, Height: 3, Zoomed: true, Boxes: []Box{{Pane: Merge, Rect: Rect{0, 0, w, 3}}}}
		line := bottom(st.Draw(l, nil))
		if ansi.StringWidth(line) != w {
			t.Errorf("width %d: bottom is %d cells", w, ansi.StringWidth(line))
		}
		b.WriteString(line + "\n")
	}
	golden.RequireEqual(t, b.String())
}

func TestStripSplit(t *testing.T) {
	st := DefaultStyles(true)
	l := Compute(Spec{Width: 100, Height: 4, Focus: Development})
	out := st.Draw(l, []Content{{Focus: true}, {}})
	line := bottom(out)
	if !strings.HasPrefix(line, "╰─ 1 Description") || !strings.HasSuffix(line, "╯") || strings.Count(line, "╯") != 1 {
		t.Errorf("the strip must run across both boxes: %q", line)
	}
	golden.RequireEqual(t, visible(out))
}

// TestStripSeam covers a seam beyond the strip, where "┴" junctions show,
// and each cell takes the colour of the box under it.
func TestStripSeam(t *testing.T) {
	st := DefaultStyles(true)
	forms := st.stripForms(Merge)
	line, ok := st.stripLine([]int{30, 30}, []bool{false, true}, forms[3])
	if !ok {
		t.Fatal("the digits fit in 60 cells")
	}
	want := "╰─ 1 2 [3] 4 5 6 " + strings.Repeat("─", 12) + "┴┴" + strings.Repeat("─", 28) + "╯"
	if plain := ansi.Strip(line); plain != want {
		t.Errorf("seam line = %q, want %q", plain, want)
	}
	golden.RequireEqual(t, visible(line))
}

// TestStripTwoBoxes hand-builds narrow split layouts: the strip runs across
// both boxes and is left out when even its shortest form does not fit.
func TestStripTwoBoxes(t *testing.T) {
	st := DefaultStyles(true)
	box := func(x, w int) Box { return Box{Pane: Merge, Role: List, Rect: Rect{x, 0, w, 3}} }
	var b strings.Builder
	for _, tt := range []struct {
		name string
		l, r int
	}{
		{"fits across", 20, 20},
		{"a seam beyond the strip", 30, 30},
		{"a strip longer than the left box", 4, 30},
		{"too narrow", 8, 8},
	} {
		l := Layout{Width: tt.l + tt.r, Height: 3, Zoomed: true, Boxes: []Box{box(0, tt.l), box(tt.l, tt.r)}}
		b.WriteString(tt.name + "\n" + ansi.Strip(st.Draw(l, nil)) + "\n\n")
	}
	golden.RequireEqual(t, b.String())
}

func TestStripOnlyZoomed(t *testing.T) {
	st := DefaultStyles(true)
	out := st.Draw(Compute(Spec{Width: 120, Height: 24, Focus: Merge}), nil)
	if strings.Contains(ansi.Strip(out), "1 2 [3]") || strings.Contains(ansi.Strip(out), "· 2 Conv") {
		t.Error("the pane strip must not show in the wide layout")
	}
}

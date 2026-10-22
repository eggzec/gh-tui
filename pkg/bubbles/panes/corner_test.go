package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestCornerSlots checks which form each slot of an edge shows. The left slot
// wins; the right slot shows only if a form fits beside it with a dash
// between; a slot that has no fitting form is left out whole.
func TestCornerSlots(t *testing.T) {
	t.Parallel()
	st := DefaultStyles(true)
	left := []string{"left slot", "left", "l"}
	right := []string{"right slot", "right", "r"}
	// An edge is "╰─ " + left + " " + dashes + " " + right + " ─╯".
	tests := []struct {
		name string
		w    int
		want string // bottom edge, without styles
	}{
		{"both slots fit in their longest form", 30, "╰─ left slot ─── right slot ─╯"},
		{"the right slot takes a shorter form", 26, "╰─ left slot ──── right ─╯"},
		{"one dash of gap is enough", 23, "╰─ left slot ─ right ─╯"},
		{"the shortest right form", 22, "╰─ left slot ──── r ─╯"},
		{"the right slot is dropped whole", 18, "╰─ left slot ────╯"},
		{"the left slot takes a shorter form first", 13, "╰─ left ────╯"},
		{"the left slot's shortest form", 6, "╰─ l ╯"},
		{"both slots are dropped", 5, "╰───╯"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ansi.Strip(st.Frame(Content{BottomLeft: left, BottomRight: right}, tt.w, 2))
			bottom := got[strings.LastIndex(got, "\n")+1:]
			if ansi.StringWidth(bottom) != tt.w {
				t.Errorf("bottom is %d cells, want %d: %q", ansi.StringWidth(bottom), tt.w, bottom)
			}
			if bottom != tt.want {
				t.Errorf("bottom = %q, want %q", bottom, tt.want)
			}
		})
	}
}

func TestCornerSlotsTop(t *testing.T) {
	t.Parallel()
	st := DefaultStyles(true)
	c := Content{TopLeft: []string{"Title"}, TopRight: []string{"a long right form", "short"}}
	for _, want := range []string{
		"╭─ Title ── a long right form ─╮",
		"╭─ Title ── short ─╮",
		"╭─ Title ──────╮",
	} {
		w := ansi.StringWidth(want)
		out := ansi.Strip(st.Frame(c, w, 2))
		got, _, _ := strings.Cut(out, "\n")
		if got != want {
			t.Errorf("w=%d: %q, want %q", w, got, want)
		}
	}
}

// TestPadding checks the one column of padding on each side of a pane, and
// that the padding stays while the content is cut: a pane's text never
// touches a side border unless the box is under 5 cells wide.
func TestPadding(t *testing.T) {
	t.Parallel()
	st := DefaultStyles(true)
	for _, tt := range []struct {
		w    int
		want string
	}{
		{7, "│ abc │"},
		{6, "│ a… │"},
		{5, "│ … │"},
		{4, "│a…│"},
	} {
		got := ansi.Strip(st.Frame(Content{Body: "abc"}, tt.w, 3))
		line := strings.Split(got, "\n")[1]
		if line != tt.want {
			t.Errorf("w=%d: %q, want %q", tt.w, line, tt.want)
		}
	}
}

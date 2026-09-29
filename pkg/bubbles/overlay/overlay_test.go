package overlay

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// frame returns h lines of w cells, each a run of one styled letter.
func frame(w, h int) string {
	colors := []string{"#7aa2f7", "#9ece6a", "#e0af68"}
	lines := make([]string, h)
	for i := range lines {
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(colors[i%len(colors)]))
		lines[i] = st.Render(strings.Repeat(string(rune('a'+i%26)), w))
	}
	return strings.Join(lines, "\n")
}

func box(text string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7aa2f7")).
		Padding(0, 1).
		Render(text)
}

func TestPlace(t *testing.T) {
	wide := strings.Repeat("你好", 10)
	tests := []struct {
		name string
		bg   string
		fg   string
		x, y int
	}{
		{name: "inside", bg: frame(30, 8), fg: box("popup"), x: 4, y: 2},
		{name: "top left", bg: frame(30, 8), fg: box("popup"), x: 0, y: 0},
		{name: "negative is clamped", bg: frame(30, 8), fg: box("popup"), x: -3, y: -2},
		{name: "cut at the edges", bg: frame(30, 8), fg: box("a wider popup"), x: 22, y: 6},
		// The box starts in the middle of a wide rune, which gives way.
		{name: "over wide runes", bg: wide + "\n" + wide + "\n" + wide, fg: "XYZ", x: 3, y: 1},
		{name: "wide runes in the box", bg: frame(20, 3), fg: "[你好]", x: 5, y: 1},
		{name: "below the frame", bg: frame(20, 3), fg: "XYZ", x: 5, y: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := Place(tt.bg, tt.fg, tt.x, tt.y)
			if got, want := lipgloss.Height(out), lipgloss.Height(tt.bg); got != want {
				t.Errorf("%d lines, want %d", got, want)
			}
			for i, l := range strings.Split(out, "\n") {
				if w := ansi.StringWidth(l); w > lipgloss.Width(tt.bg) {
					t.Errorf("line %d is %d wide, wider than the frame", i, w)
				}
			}
			golden.RequireEqual(t, out)
		})
	}
}

func TestCenter(t *testing.T) {
	tests := []struct {
		name string
		w, h int
		fg   string
	}{
		{name: "centered", w: 30, h: 9, fg: box("popup")},
		{name: "larger than the frame", w: 10, h: 3, fg: box("a popup that is too wide\nand\ntoo\ntall")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			golden.RequireEqual(t, Center(frame(tt.w, tt.h), tt.fg, tt.w, tt.h))
		})
	}
}

// The frame keeps its styles on both sides of the box.
func TestPlaceKeepsStyles(t *testing.T) {
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	bg := red.Render(strings.Repeat("r", 10))
	out := Place(bg, "XY", 4, 0)
	if got := ansi.Strip(out); got != "rrrrXYrrrr" {
		t.Fatalf("text = %q", got)
	}
	left, right, _ := strings.Cut(out, "XY")
	for _, part := range []string{left, right} {
		if !strings.Contains(part, "38;2;255;0;0") {
			t.Errorf("%q lost the frame's color", part)
		}
	}
}

// A link that the box cuts closes before the box and opens again after
// it, and no link closes that isn't open.
func TestPlaceKeepsLinksWhole(t *testing.T) {
	link := ansi.SetHyperlink("https://github.com/cli/cli") + "link" + ansi.ResetHyperlink()
	for _, bg := range []string{"ab" + link + "cd", link + "abcdefgh", "abcdefgh" + link} {
		out := Place(bg, "XY", 4, 0)
		open := false
		for _, part := range strings.Split(out, "\x1b]8;")[1:] {
			_, rest, _ := strings.Cut(part, ";")
			opens := rest != "" && rest[0] != '\x1b' && rest[0] != '\a'
			if !opens && !open {
				t.Errorf("Place(%q) = %q: a link closes that isn't open", bg, out)
			}
			open = opens
		}
		if open {
			t.Errorf("Place(%q) = %q: a link stays open", bg, out)
		}
		if before, _, _ := strings.Cut(out, "XY"); linkOpen(before) {
			t.Errorf("Place(%q) = %q: the box is inside the link", bg, out)
		}
	}
}

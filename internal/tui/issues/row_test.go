package issues

import (
	"image/color"
	"testing"
)

func TestLayoutDropsColumns(t *testing.T) {
	tests := []struct {
		width int
		want  columns
	}{
		{118, columns{chips: 2, comments: true, author: true, age: true}},
		{78, columns{chips: 1, comments: true, author: true, age: true}},
		{66, columns{chips: 1, comments: true, age: true}},
		{58, columns{chips: 1, age: true}},
		{40, columns{age: true}},
		{20, columns{}},
	}
	for _, tt := range tests {
		got := layout(tt.width)
		if got.title < minTitle && got != (columns{title: got.title}) {
			t.Errorf("layout(%d) leaves the title %d cells", tt.width, got.title)
		}
		got.title, got.labels = 0, 0
		if got != tt.want {
			t.Errorf("layout(%d) = %+v, want %+v", tt.width, got, tt.want)
		}
	}
}

func TestChipColors(t *testing.T) {
	for _, hex := range []string{"", "zzzzzz", "12345", "#12"} {
		if _, _, ok := chipColors(hex, true); ok {
			t.Errorf("chipColors(%q) accepted an invalid color", hex)
		}
	}
	lum := func(c color.Color) uint32 {
		r, g, b, _ := c.RGBA()
		return (r + g + b) / 3 >> 8
	}
	// A loud yellow and a near black both come out legible and quiet.
	for _, hex := range []string{"fbca04", "#000", "d73a4a", "ffffff"} {
		fg, bg, ok := chipColors(hex, true)
		if !ok {
			t.Fatalf("chipColors(%q) rejected a valid color", hex)
		}
		if lum(fg) < 150 || lum(bg) > 90 {
			t.Errorf("dark chip of %s: fg %d, bg %d; want a light fg on a dim bg", hex, lum(fg), lum(bg))
		}
		fg, bg, _ = chipColors(hex, false)
		if lum(fg) > 110 || lum(bg) < 200 {
			t.Errorf("light chip of %s: fg %d, bg %d; want a dark fg on a pale bg", hex, lum(fg), lum(bg))
		}
	}
}

package ui

import (
	"image/color"
	"testing"
)

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

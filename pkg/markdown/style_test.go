package markdown

import (
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// TestLightCodeReads checks that every color of code on a light terminal
// has the contrast that WCAG asks of text, 4.5:1, on white, and inline code
// on its own background.
func TestLightCodeReads(t *testing.T) {
	s := DefaultStyle(false)
	white := lipgloss.Color("#ffffff")
	v := reflect.ValueOf(*s.CodeBlock.Chroma)
	for i := range v.NumField() {
		p := v.Field(i).Interface().(ansi.StylePrimitive)
		if p.Color == nil {
			continue
		}
		bg := white
		if p.BackgroundColor != nil {
			bg = lipgloss.Color(*p.BackgroundColor)
		}
		if r := contrast(lipgloss.Color(*p.Color), bg); r < 4.5 {
			t.Errorf("%s %s has a contrast of %.1f, want 4.5 or more", v.Type().Field(i).Name, *p.Color, r)
		}
	}
	// Code sits on the terminal's own background.
	if bg := s.CodeBlock.Chroma.Background; bg.BackgroundColor != nil || bg.Color != nil {
		t.Errorf("code has a background of its own: %+v", bg)
	}
	if r := contrast(lipgloss.Color(*s.Code.Color), lipgloss.Color(*s.Code.BackgroundColor)); r < 4.5 {
		t.Errorf("inline code has a contrast of %.1f, want 4.5 or more", r)
	}
}

// contrast is the WCAG contrast ratio of a and b.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// A title holding a word too long for a line, such as an address, is
// broken within the width, in either style.
func TestTitlesFitTheWidth(t *testing.T) {
	for _, dark := range []bool{true, false} {
		r := New(DefaultStyle(dark))
		for _, src := range []string{"# " + strings.Repeat("x", 100), "# https://example.com/" + strings.Repeat("path/", 30)} {
			for _, width := range []int{8, 20, 40, 80} {
				for l := range strings.SplitSeq(r.Render(src, width), "\n") {
					if w := xansi.StringWidth(l); w > width {
						t.Errorf("dark %t at %d cells: a line of %.20q… is %d wide: %q", dark, width, src, w, xansi.Strip(l))
					}
				}
			}
		}
	}
}

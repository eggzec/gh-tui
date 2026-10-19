package ui

import (
	"image/color"
	"math"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// labelChip is a rendered label and its width in cells.
type labelChip struct {
	text  string
	width int
}

// maxLabelChips bounds the chip cache. Repositories rarely have this many
// labels, and clearing it only costs rendering them again.
const maxLabelChips = 512

// LabelChips renders labels as colored chips and keeps the chips it
// rendered, by name and color. Use one per list or modal, and make a new one
// when the styles change.
type LabelChips struct {
	chips map[string]labelChip
	// label is the chip of a label whose color is not valid.
	label lipgloss.Style
	dark  bool
	// ellipsis ends a name cut short, and nameWidth is the longest name a
	// chip shows.
	ellipsis  string
	nameWidth int
}

// NewLabelChips returns chips that draw a label whose color is not valid
// with label, tune the others to a dark or a light terminal background, and
// cut names to nameWidth cells, ending them with ellipsis.
func NewLabelChips(label lipgloss.Style, dark bool, ellipsis string, nameWidth int) LabelChips {
	return LabelChips{chips: map[string]labelChip{}, label: label, dark: dark, ellipsis: ellipsis, nameWidth: nameWidth}
}

// Get returns the chip of l and its width in cells, rendering it the first
// time it is shown.
func (cc *LabelChips) Get(l core.Label) (text string, width int) {
	return cc.get(l, false)
}

// GetBold is Get for the chip of a selected row, which is bold like its other
// cells. It takes the same width.
func (cc *LabelChips) GetBold(l core.Label) (text string, width int) {
	return cc.get(l, true)
}

func (cc *LabelChips) get(l core.Label, bold bool) (text string, width int) {
	k := l.Name + "\x00" + l.Color
	if bold {
		k += "\x00b"
	}
	if c, ok := cc.chips[k]; ok {
		return c.text, c.width
	}
	if len(cc.chips) >= maxLabelChips {
		clear(cc.chips)
	}
	name := termtext.Truncate(OneLine(l.Name), cc.nameWidth, cc.ellipsis)
	st := cc.label
	if fg, bg, ok := chipColors(l.Color, cc.dark); ok {
		st = lipgloss.NewStyle().Foreground(fg).Background(bg).Padding(0, 1)
	}
	if bold {
		st = st.Bold(true)
	}
	text = st.Render(name)
	c := labelChip{text: text, width: ansi.StringWidth(text)}
	cc.chips[k] = c
	return c.text, c.width
}

// Tint returns base with the colors of the chip of a label whose color is hex,
// for text that names the label outside a chip. It returns base if hex is not
// a color.
func (cc *LabelChips) Tint(base lipgloss.Style, hex string) lipgloss.Style {
	fg, bg, ok := chipColors(hex, cc.dark)
	if !ok {
		return base
	}
	return base.Foreground(fg).Background(bg)
}

// chipColors turns a label's hex color into a quiet chip: its hue with the
// saturation capped, on a dim background of the same hue. The lightness is
// fixed per terminal background, so every label stays legible and none
// shouts. It reports false if hex is not a color.
func chipColors(hex string, dark bool) (fg, bg color.Color, ok bool) {
	h, s, _, ok := parseHSL(hex)
	if !ok {
		return nil, nil, false
	}
	if dark {
		return hsl(h, min(s, 0.55), 0.78), hsl(h, min(s, 0.35), 0.22), true
	}
	return hsl(h, min(s, 0.6), 0.30), hsl(h, min(s, 0.45), 0.90), true
}

// parseHSL parses a hex color such as "d73a4a", with or without '#', into
// hue in degrees, saturation and lightness.
func parseHSL(hex string) (h, s, l float64, ok bool) {
	if hex != "" && hex[0] == '#' {
		hex = hex[1:]
	}
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	r := float64(v>>16&0xff) / 255
	g := float64(v>>8&0xff) / 255
	b := float64(v&0xff) / 255

	hi, lo := max(r, g, b), min(r, g, b)
	l = (hi + lo) / 2
	d := hi - lo
	if d == 0 {
		return 0, 0, l, true
	}
	s = d / (1 - math.Abs(2*l-1))
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l, true
}

// hsl returns the color of hue h in degrees, saturation s and lightness l.
func hsl(h, s, l float64) color.Color {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	to := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return color.RGBA{R: to(r), G: to(g), B: to(b), A: 0xff}
}

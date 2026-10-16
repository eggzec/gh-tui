package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// stripForm is the pane strip written one way: styled text and its width.
type stripForm struct {
	text  string
	width int
}

// stripForms are the strip from the longest form to the shortest: full
// names, short names, short names without separators, then digits with the
// current pane in brackets. The current pane is accented, the others dim.
func (s Styles) stripForms(cur Pane) []stripForm {
	build := func(name func(Pane) string, sep string, digits bool) stripForm {
		parts := make([]string, 0, Count)
		for p := Description; p <= Files; p++ {
			var t string
			switch {
			case digits && p == cur:
				t = fmt.Sprintf("[%d]", p)
			case digits:
				t = fmt.Sprintf("%d", p)
			default:
				t = fmt.Sprintf("%d %s", p, name(p))
			}
			if p == cur {
				parts = append(parts, s.StripCurrent.Render(t))
			} else {
				parts = append(parts, s.StripOther.Render(t))
			}
		}
		text := strings.Join(parts, s.StripSep.Render(sep))
		return stripForm{text: text, width: ansi.StringWidth(text)}
	}
	return []stripForm{
		build(Pane.Name, " · ", false),
		build(Pane.Short, " · ", false),
		build(Pane.Short, " ", false),
		build(nil, " ", true),
	}
}

// stripLine is one bottom border line over boxes of the given widths, side
// by side, that carries the strip: "╰─ strip ─────╯", with "┴" where two
// boxes meet beyond the strip, and each cell in the colour of the box it
// lies under. It fails when the strip and its " ─╯" do not fit.
func (s Styles) stripLine(widths []int, focus []bool, f stripForm) (string, bool) {
	total := 0
	for _, w := range widths {
		total += w
	}
	end := 3 + f.width + 1 // the cells up to the dashes after the strip
	if end+2 > total {
		return "", false
	}
	// seam[i] is set for the two cells where boxes meet.
	seam := make([]bool, total)
	at := 0
	for _, w := range widths[:len(widths)-1] {
		at += w
		seam[at-1], seam[at] = true, true
	}
	box, edge := 0, widths[0]
	cell := func(i int, g string) string {
		for i >= edge {
			box++
			edge += widths[box]
		}
		return s.edge(focus[box]).Render(g)
	}
	b := s.Border
	var out strings.Builder
	out.WriteString(cell(0, b.BottomLeft+b.Bottom+" "))
	out.WriteString(f.text)
	out.WriteString(cell(3+f.width, " "))
	for i := end; i < total; i++ {
		switch {
		case i == total-1:
			out.WriteString(cell(i, b.BottomRight))
		case seam[i]:
			out.WriteString(cell(i, b.MiddleBottom))
		default:
			out.WriteString(cell(i, b.Bottom))
		}
	}
	return out.String(), true
}

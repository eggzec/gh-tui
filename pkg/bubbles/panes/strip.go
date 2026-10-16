package panes

import (
	"fmt"
	"iter"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// stripForm is the pane strip written one way: styled text and its width.
type stripForm struct {
	text  string
	width int
}

// stripForms yields the strip from the longest form to the shortest: full
// names, short names, short names without separators, then digits with the
// current pane in brackets. The current pane is accented, the others dim.
// Each form is built when the loop asks for it, so a loop that stops at the
// first form that fits builds no more.
func (s Styles) stripForms(cur Pane) iter.Seq[stripForm] {
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
	return func(yield func(stripForm) bool) {
		for _, f := range []func() stripForm{
			func() stripForm { return build(Pane.Name, " · ", false) },
			func() stripForm { return build(Pane.Short, " · ", false) },
			func() stripForm { return build(Pane.Short, " ", false) },
			func() stripForm { return build(nil, " ", true) },
		} {
			if !yield(f()) {
				return
			}
		}
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
	// Glyphs gather in a run while the colour stays the same, and each run
	// is rendered once; cells are visited from left to right.
	box, edge := 0, widths[0]
	var out, run strings.Builder
	runFocus := focus[0]
	flush := func() {
		if run.Len() > 0 {
			out.WriteString(s.edge(runFocus).Render(run.String()))
			run.Reset()
		}
	}
	cell := func(i int, g string) {
		for i >= edge {
			box++
			edge += widths[box]
		}
		if focus[box] != runFocus {
			flush()
			runFocus = focus[box]
		}
		run.WriteString(g)
	}
	b := s.Border
	cell(0, b.BottomLeft+b.Bottom+" ")
	flush() // the strip has colours of its own
	out.WriteString(f.text)
	cell(3+f.width, " ")
	for i := end; i < total; i++ {
		switch {
		case i == total-1:
			cell(i, b.BottomRight)
		case seam[i]:
			cell(i, b.MiddleBottom)
		default:
			cell(i, b.Bottom)
		}
	}
	flush()
	return out.String(), true
}

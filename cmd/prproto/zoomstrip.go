package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// zoomForms are the strip of panes with their digits, from the full names to
// the digits only; the current pane is accented.
func (m *model) zoomForms() []string {
	short := []string{"Desc", "Conv", "Merge", "Inv", "Dev", "Files"}
	build := func(names []string, sep string, digits bool) string {
		var out []string
		for i := range paneNames {
			var s string
			switch {
			case digits && i == m.focus:
				s = fmt.Sprintf("[%d]", i+1)
			case digits:
				s = fmt.Sprintf("%d", i+1)
			default:
				s = fmt.Sprintf("%d %s", i+1, names[i])
			}
			if i == m.focus {
				s = bold(accent(s))
			} else {
				s = dim(s)
			}
			out = append(out, s)
		}
		return strings.Join(out, dim(sep))
	}
	return []string{build(paneNames, " · ", false), build(short, " · ", false), build(short, " ", false), build(nil, " ", true)}
}

// fitStrip puts the strip on the bottom border line of a box, if it fits.
func fitStrip(box, strip string, focus bool) (string, bool) {
	ls := strings.Split(box, "\n")
	last := ls[len(ls)-1]
	if !strings.HasPrefix(ansi.Strip(last), "╰") {
		return box, false
	}
	dashes := ansi.StringWidth(last) - 5 - ansi.StringWidth(strip)
	if dashes < 0 {
		return box, false
	}
	col := dim
	if focus {
		col = accent
	}
	ls[len(ls)-1] = col("╰─ ") + strip + col(" "+strings.Repeat("─", dashes)+"╯")
	return strings.Join(ls, "\n"), true
}

// spanStrip puts the strip on one bottom border line across two boxes side by
// side, meeting where they do.
func spanStrip(l, r string, strip string, fl, fr bool) (string, bool) {
	ls, rs := strings.Split(l, "\n"), strings.Split(r, "\n")
	ll, rl := ls[len(ls)-1], rs[len(rs)-1]
	if !strings.HasPrefix(ansi.Strip(ll), "╰") || !strings.HasPrefix(ansi.Strip(rl), "╰") {
		return "", false
	}
	wl, wr := ansi.StringWidth(ll), ansi.StringWidth(rl)
	sw := ansi.StringWidth(strip)
	end := 3 + sw + 1 // the cells up to the dashes after the strip
	if end > wl+wr-1 {
		return "", false
	}
	paint := func(i int) func(string) string {
		if (i < wl && fl) || (i >= wl && fr) {
			return accent
		}
		return dim
	}
	var b strings.Builder
	cell := func(i int, s string) { b.WriteString(paint(i)(s)) }
	cell(0, "╰─ ")
	b.WriteString(strip)
	cell(3+sw, " ")
	for i := end; i < wl+wr; i++ {
		switch {
		case i == wl+wr-1:
			cell(i, "╯")
		case i == wl-1 || i == wl:
			cell(i, "┴")
		default:
			cell(i, "─")
		}
	}
	return b.String(), true
}

// zoomStrip joins the boxes of the zoomed view and draws the pane strip on its
// bottom border: the longest form that fits, across the whole width if there
// are two boxes, else on the last box that has room for it.
func (m *model) zoomStrip(boxes []string, focus []bool) string {
	join := func() string { return lipgloss.JoinHorizontal(lipgloss.Top, boxes...) }
	for _, f := range m.zoomForms() {
		if len(boxes) == 2 {
			if line, ok := spanStrip(boxes[0], boxes[1], f, focus[0], focus[1]); ok {
				ls := strings.Split(join(), "\n")
				ls[len(ls)-1] = line
				return strings.Join(ls, "\n")
			}
		}
		for i := len(boxes) - 1; i >= 0; i-- {
			if s, ok := fitStrip(boxes[i], f, focus[i]); ok {
				boxes[i] = s
				return join()
			}
		}
	}
	return join()
}

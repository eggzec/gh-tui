package panes

import "strings"

// Draw frames the content of each box of the layout and joins the boxes
// into the area: exactly Height lines of Width cells. Content i belongs to
// box i of l.Boxes; a missing one is an empty box.
//
// A zoomed layout carries the pane strip on its bottom border, the longest
// form that fits, run as one line across both boxes when the view is split.
// Both boxes together are never narrower than one of them, so a strip that
// does not fit across them fits on neither alone, and is left out.
func (s Styles) Draw(l Layout, content []Content) string {
	type drawn struct {
		box   Box
		focus bool
		lines []string
	}
	var boxes []drawn
	for i, b := range l.Boxes {
		if b.Rect.W <= 0 || b.Rect.H <= 0 {
			continue
		}
		var c Content
		if i < len(content) {
			c = content[i]
		}
		if c.Title == "" && b.Role != Preview {
			c.Title = b.Pane.Title()
		}
		boxes = append(boxes, drawn{b, c.Focus, s.frame(c, b.Rect.W, b.Rect.H)})
	}
	if l.Zoomed && len(boxes) > 0 && boxes[0].box.Rect.H >= 2 {
		widths, focus := make([]int, len(boxes)), make([]bool, len(boxes))
		for i, d := range boxes {
			widths[i], focus[i] = d.box.Rect.W, d.focus
		}
		cur := boxes[0].box.Pane
		for _, f := range s.stripForms(cur) {
			line, ok := s.stripLine(widths, focus, f)
			if !ok {
				continue
			}
			last := len(boxes[0].lines) - 1
			boxes[0].lines[last] = line
			for _, d := range boxes[1:] {
				d.lines[last] = ""
			}
			break
		}
	}
	rows := make([]strings.Builder, l.Height)
	for _, d := range boxes {
		for r, line := range d.lines {
			rows[d.box.Rect.Y+r].WriteString(line)
		}
	}
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].String()
	}
	return strings.Join(out, "\n")
}

package panes

import "strings"

// Draw frames the content of each box of the layout and joins the boxes
// into the area: exactly Height lines of Width cells. It asks content for
// each box that has cells, so the caller picks by the box's Pane and Role and
// cannot misalign the content with the layout; a nil content draws empty
// boxes. Draw adds nothing of its own: the caller decides in Content which
// box is focused and what each is titled.
func (s Styles) Draw(l Layout, content func(Box) Content) string {
	rows := make([]strings.Builder, l.Height)
	for _, b := range l.Boxes {
		if b.Rect.W <= 0 || b.Rect.H <= 0 {
			continue
		}
		var c Content
		if content != nil {
			c = content(b)
		}
		for r, line := range s.frame(c, b.Rect.W, b.Rect.H) {
			rows[b.Rect.Y+r].WriteString(line)
		}
	}
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].String()
	}
	return strings.Join(out, "\n")
}

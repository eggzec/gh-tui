package panes

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Content is what the caller puts in one box.
type Content struct {
	// Title is the top-left slot of the border. It is the one slot that is
	// cut, with an ellipsis, since a title that does not fit is still better
	// shown in part; an empty Title shows none.
	Title string
	// TopRight, BottomLeft and BottomRight are the other three corner slots
	// of the border. Each is a list of forms of the same text, written by
	// the caller, the longest first. The bubble draws the first form that
	// fits and never edits, cuts or pads it; a slot with no fitting form is
	// left out whole. On one edge the left slot wins and takes its longest
	// form that fits, and the right slot appears only if one of its forms
	// fits beside it with at least one dash between them. Colours inside a
	// form are the caller's; the Info style wraps the form.
	TopRight, BottomLeft, BottomRight []string
	// Body is the content, lines joined by "\n". Lines beyond the inner
	// height are dropped, and lines wider than the inner width are cut with
	// an ellipsis. Tabs are expanded to stops of 4. Other control characters,
	// such as "\r", are not removed and break the widths: the caller strips
	// them.
	Body string
	// Focus draws the border in the focus colour. The caller decides which
	// box is focused.
	Focus bool
}

// Frame draws one box of w by h cells around c: a titled top border, the
// body between side borders with one column of padding, and a bottom
// border. The result is exactly h lines of w cells; a box under 2 cells in
// either direction is blank.
func (s Styles) Frame(c Content, w, h int) string {
	return strings.Join(s.frame(c, w, h), "\n")
}

func (s Styles) frame(c Content, w, h int) []string {
	if w < 2 || h < 2 {
		blank := make([]string, max(h, 0))
		for i := range blank {
			blank[i] = strings.Repeat(" ", max(w, 0))
		}
		return blank
	}
	b, e := s.Border, s.edge(c.Focus)
	side, right := e.Render(b.Left), e.Render(b.Right)
	lines := make([]string, 0, h)
	lines = append(lines, s.top(c, w))
	inner := w - 2
	body := strings.Split(c.Body, "\n")
	for i := range h - 2 {
		var line string
		if i < len(body) {
			line = expandTabs(body[i])
		}
		lines = append(lines, side+s.pad(line, inner)+right)
	}
	return append(lines, s.bottom(c, w))
}

// bottom is the bottom border: corner, "─ ", left slot, " ", dashes, " ",
// right slot, " ─", corner. The right slot goes only if at least one dash is
// left between it and the left slot.
func (s Styles) bottom(c Content, w int) string {
	b, e := s.Border, s.edge(c.Focus)
	inner := w - 2
	left, leftW := "", 0
	for _, f := range c.BottomLeft {
		if fw := ansi.StringWidth(f); f != "" && 3+fw <= inner {
			left, leftW = e.Render(b.Bottom+" ")+s.Info.Render(f)+e.Render(" "), 3+fw
			break
		}
	}
	right, rightW := s.rightSlot(c.BottomRight, b.Bottom, e, inner-leftW)
	if leftW+rightW == 0 {
		return e.Render(b.BottomLeft + strings.Repeat(b.Bottom, inner) + b.BottomRight)
	}
	dashes := e.Render(strings.Repeat(b.Bottom, inner-leftW-rightW))
	return e.Render(b.BottomLeft) + left + dashes + right + e.Render(b.BottomRight)
}

// rightSlot is the first of forms that fits in room cells with a dash before
// it, drawn as " form ─" with the dash glyph of the edge, and its width.
func (s Styles) rightSlot(forms []string, dash string, e lipgloss.Style, room int) (slot string, width int) {
	for _, f := range forms {
		if fw := ansi.StringWidth(f); f != "" && fw+3+1 <= room {
			return e.Render(" ") + s.Info.Render(f) + e.Render(" "+dash), fw + 3
		}
	}
	return "", 0
}

// pad fits a line into the cells between the side borders: one column of
// padding on each side when there is room for it.
func (s Styles) pad(line string, inner int) string {
	margin := 0
	if inner >= 3 {
		margin = 1
	}
	room := inner - 2*margin
	line = s.cut(line, room)
	gap := room - ansi.StringWidth(line)
	return strings.Repeat(" ", margin) + line + strings.Repeat(" ", gap+margin)
}

// top is the top border: corner, "─ ", title, " ", dashes, " ", info, " ─",
// corner. The info goes only if at least one dash is left between it and
// the title.
func (s Styles) top(c Content, w int) string {
	b, e := s.Border, s.edge(c.Focus)
	inner := w - 2
	title := c.Title
	left := ""
	leftW := 0
	if title != "" {
		room := inner - 3
		if room > 0 {
			title = s.cut(title, room)
			ts := s.Title
			if c.Focus {
				ts = s.TitleFocus
			}
			left = e.Render(b.Top+" ") + ts.Render(title) + e.Render(" ")
			leftW = 3 + ansi.StringWidth(title)
		}
	}
	info, infoW := s.rightSlot(c.TopRight, b.Top, e, inner-leftW)
	dashes := e.Render(strings.Repeat(b.Top, inner-leftW-infoW))
	return e.Render(b.TopLeft) + left + dashes + info + e.Render(b.TopRight)
}

// cut cuts a line to w cells with an ellipsis, and seals it.
func (s Styles) cut(line string, w int) string {
	if ansi.StringWidth(line) > w {
		line = ansi.Truncate(line, w, s.Ellipsis)
	}
	return seal(line)
}

// seal closes the hyperlink and the style a line leaves open, cut or not, so
// that neither runs on into the border and the padding after it.
func seal(line string) string {
	if !strings.Contains(line, "\x1b") {
		return line
	}
	if linkOpen(line) {
		line += "\x1b]8;;\x1b\\"
	}
	if sgrOpen(line) {
		line += "\x1b[m"
	}
	return line
}

// sgrOpen reports whether s ends with a style that was set and not reset.
func sgrOpen(s string) bool {
	open := false
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			return open
		}
		s = s[i+2:]
		end := strings.IndexFunc(s, func(r rune) bool { return r >= 0x40 && r <= 0x7e })
		if end < 0 {
			return open
		}
		if s[end] == 'm' {
			// "m" and "0m" reset; anything with another digit sets something.
			// "1;0" counts as open, which only costs a reset too many.
			open = strings.ContainsAny(s[:end], "123456789")
		}
		s = s[end+1:]
	}
}

// linkOpen reports whether s ends inside an OSC 8 hyperlink.
func linkOpen(s string) bool {
	open := false
	for {
		i := strings.Index(s, "\x1b]8;")
		if i < 0 {
			return open
		}
		s = s[i+4:]
		end := len(s)
		for _, st := range []string{"\x1b\\", "\a"} {
			if j := strings.Index(s, st); j >= 0 {
				end = min(end, j)
			}
		}
		params := s[:end]
		_, url, _ := strings.Cut(params, ";")
		open = url != ""
		s = s[end:]
	}
}

// expandTabs replaces each tab with spaces up to the next stop of 4,
// counting cells so that escape sequences and wide characters do not move
// the stops.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	parts := strings.Split(s, "\t")
	var b strings.Builder
	col := 0
	for i, p := range parts {
		b.WriteString(p)
		col += ansi.StringWidth(p)
		if i < len(parts)-1 {
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
		}
	}
	return b.String()
}

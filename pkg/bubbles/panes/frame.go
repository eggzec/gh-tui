package panes

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Content is what the caller puts in one box.
type Content struct {
	// Title is the left part of the top border. When empty, a box that is
	// not a Preview shows its pane's Title.
	Title string
	// Info are the forms of the right-hand info of the top border, the
	// longest first. The first that fits is drawn, and none when none
	// does; a form is never cut. Colours inside a form are the caller's;
	// the Info style wraps the form.
	Info []string
	// Body is the content, lines joined by "\n". Lines beyond the inner
	// height are dropped, and lines wider than the inner width are cut with
	// an ellipsis. Tabs are expanded to stops of 4.
	Body string
	// Focus draws the border in the focus colour.
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
	side := e.Render(b.Left)
	lines := make([]string, 0, h)
	lines = append(lines, s.top(c, w))
	inner := w - 2
	body := strings.Split(c.Body, "\n")
	for i := range h - 2 {
		var line string
		if i < len(body) {
			line = expandTabs(body[i])
		}
		lines = append(lines, side+s.pad(line, inner)+e.Render(b.Right))
	}
	return append(lines, e.Render(b.BottomLeft+strings.Repeat(b.Bottom, inner)+b.BottomRight))
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
	info, infoW := "", 0
	for _, f := range c.Info {
		fw := ansi.StringWidth(f)
		if f != "" && leftW+fw+3+1 <= inner {
			info, infoW = e.Render(" ")+s.Info.Render(f)+e.Render(" "+b.Top), fw+3
			break
		}
	}
	dashes := e.Render(strings.Repeat(b.Top, inner-leftW-infoW))
	return e.Render(b.TopLeft) + left + dashes + info + e.Render(b.TopRight)
}

// cut cuts a line to w cells with an ellipsis, and closes a hyperlink the
// cut left open so that it does not run on into the border.
func (s Styles) cut(line string, w int) string {
	if ansi.StringWidth(line) <= w {
		return line
	}
	out := ansi.Truncate(line, w, s.Ellipsis)
	if linkOpen(out) {
		out += "\x1b]8;;\x1b\\"
	}
	return out
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

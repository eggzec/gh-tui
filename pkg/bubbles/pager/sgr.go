package pager

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// styleLines splits styles, the colors of the text lines were split from,
// by line, with their positions in it. A line whose start isn't styled by
// a style of its own starts with the style the lines before it left, so
// it shows the same colors whichever lines are shown around it.
func styleLines(lines []string, styles []termtext.Style) [][]termtext.Style {
	out := make([][]termtext.Style, len(lines))
	// The styles of every line are slices of flat, so a line costs no
	// allocation of its own. What a line carries sits at the end of flat,
	// so the next line with colors starts with it.
	flat := make([]termtext.Style, 0, 2*len(styles))
	var carry []termtext.Style
	start, k := 0, 0
	for i, l := range lines {
		end := start + len(l)
		if k == len(styles) || styles[k].Pos > end {
			// Lines without colors of their own share what they carry.
			out[i] = carry
			start = end + 1
			continue
		}
		from := len(flat) - len(carry)
		if styles[k].Pos == start {
			// The line's own style at its start replaces what it carries.
			from = len(flat)
		}
		for ; k < len(styles) && styles[k].Pos <= end; k++ {
			st := styles[k]
			st.Pos -= start
			flat = append(flat, st)
		}
		line := flat[from:len(flat):len(flat)]
		out[i] = line
		carry = nil
		if seq := line[len(line)-1].Seq; seq != "" {
			flat = append(flat, termtext.Style{Seq: seq})
			carry = flat[len(flat)-1 : len(flat) : len(flat)]
		}
		start = end + 1
	}
	return out
}

// writeStyled writes bytes a to e of line i, whose content has colors of
// its own, in them over the text style, or in the style of the matches
// over them, which hide its colors. It writes one style at most for each
// byte, each short, so a frame stays small whatever the content holds.
func (m Model) writeStyled(b *strings.Builder, i, a, e int) {
	s, base, marks := m.lines[i], m.esc.text, m.sgr[i]
	matches, first, _ := m.lineHits(i)
	cur := -1
	if m.search.cur >= 0 && i == m.search.curLine {
		cur = m.search.curNth
	}
	// k counts the styles at or before a, the last of which is the style
	// there.
	k, _ := slices.BinarySearchFunc(marks, a+1, func(x termtext.Style, pos int) int { return x.Pos - pos })
	style := ""
	if k > 0 {
		style = marks[k-1].Seq
	}
	// The pen writes only what changes from one style to the next.
	pen := base.pen
	pen.Start(b)
	pen.Write(b, style)
	mi, inMatch := 0, false
	for pos := a; pos < e; {
		if k < len(marks) && marks[k].Pos <= pos {
			for k < len(marks) && marks[k].Pos <= pos {
				k++
			}
			style = marks[k-1].Seq
			// A match hides the colors, which come back after it.
			if !inMatch {
				pen.Write(b, style)
			}
		}
		for mi < len(matches) && matches[mi][1] <= pos {
			mi++
		}
		next := e
		if k < len(marks) {
			next = min(next, marks[k].Pos)
		}
		if mi < len(matches) {
			start, end := matches[mi][0], matches[mi][1]
			if start > pos {
				next = min(next, start)
			} else {
				if !inMatch {
					p := m.esc.match
					if first+mi == cur {
						p = m.esc.current
					}
					b.WriteString(ansi.ResetStyle)
					b.WriteString(p.on)
					pen.Lost()
					inMatch = true
				}
				next = min(next, end)
			}
		}
		b.WriteString(s[pos:next])
		pos = next
		if inMatch && pos >= matches[mi][1] {
			pen.Write(b, style)
			inMatch = false
		}
	}
	b.WriteString(ansi.ResetStyle)
}

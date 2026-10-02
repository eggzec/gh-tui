package toast

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// minWidth keeps short messages readable in narrow layouts.
	minWidth = 24
)

// View renders the stack, newest at the bottom, as a block whose toasts
// share one width and are aligned to its right edge. The block is as wide as
// the widest room of its toasts allows at most, and never wider than the
// width. It is empty when there are no toasts.
func (m Model) View() string { return m.view }

// Overlay draws the stack over the bottom-right corner of a background of
// the given size, within the inset, and returns the result, which has
// exactly height lines. Styled content in the background keeps its escape
// sequences intact. The stack is meant for a background of the size set
// with SetSize, with the inset.
func (m Model) Overlay(background string, width, height int) string {
	right, bottom := m.inset[0], m.inset[1]
	room := height - bottom
	if m.view == "" || width <= 0 || room <= 0 {
		return background
	}
	rows := strings.Split(background, "\n")
	rows = rows[:min(len(rows), height)]
	for len(rows) < height {
		rows = append(rows, "")
	}
	stack := m.view
	if n := strings.Count(stack, "\n") + 1; n > room {
		// Keep the newest, at the bottom.
		stack = stack[nthNewline(stack, n-room)+1:]
	}
	fg := lipgloss.NewLayer(stack)
	// Only the rows under the stack need a canvas; the rest pass through.
	top := room - fg.Height()
	band := rows[top:room]
	bg := lipgloss.NewLayer(strings.Join(band, "\n"))
	fg.X(max(width-right-fg.Width(), 0)).Z(1)
	composed := lipgloss.NewCanvas(width, len(band)).
		Compose(lipgloss.NewCompositor(bg, fg)).
		Render()
	out := append(slices.Clip(rows[:top]), composed)
	return strings.Join(append(out, rows[room:]...), "\n")
}

// nthNewline returns the index of the nth newline in s, counting from one.
func nthNewline(s string, n int) int {
	i := -1
	for range n {
		i += strings.IndexByte(s[i+1:], '\n') + 1
	}
	return i
}

func (m Model) render() string {
	if len(m.toasts) == 0 {
		return ""
	}
	d := m.derived
	// The inner width holds the glyph, a space, the text and the count,
	// within the room of each toast's level.
	inner := 0
	for _, t := range m.toasts {
		need := d.glyphWidth + 1 + ansi.StringWidth(t.text) + m.countWidth(t.count)
		inner = max(inner, min(need, m.maxInner(t.level)))
	}
	if inner < d.glyphWidth+2 {
		// Too narrow for any text; show nothing rather than break the layout.
		return ""
	}

	blocks := make([]string, 0, len(m.toasts))
	lines := 0
	// Walk from the newest so that a short height keeps the newest toasts.
	for _, t := range slices.Backward(m.toasts) {
		b := m.renderToast(t, inner)
		h := strings.Count(b, "\n") + 1
		if m.height > 0 && lines+h > m.height {
			break
		}
		lines += h
		blocks = append(blocks, b)
	}
	slices.Reverse(blocks)
	return strings.Join(blocks, "\n")
}

// renderToast renders one toast whose content is exactly inner cells wide.
func (m Model) renderToast(t toast, inner int) string {
	d := m.derived
	ls := m.styles.level(t.level)
	count := ""
	if t.count > 1 {
		count = " " + m.styles.Times + strconv.Itoa(t.count)
	}
	textWidth := inner - d.glyphWidth - 1 - ansi.StringWidth(count)
	if textWidth < 1 {
		// No room for the count; the text matters more.
		count = ""
		textWidth = inner - d.glyphWidth - 1
	}
	wrapped, _ := wrap(t.text, textWidth, m.rooms[t.level].Lines, m.styles.Ellipsis)

	glyph := d.glyph[t.level].Render(padRight(ls.Glyph, d.glyphWidth))
	blank := d.text.Render(strings.Repeat(" ", d.glyphWidth))
	var sb strings.Builder
	for i, line := range wrapped {
		if i > 0 {
			sb.WriteByte('\n')
			sb.WriteString(blank)
		} else {
			sb.WriteString(glyph)
		}
		sb.WriteString(d.text.Render(" " + padRight(line, textWidth)))
		if count != "" {
			if i == 0 {
				sb.WriteString(d.count.Render(count))
			} else {
				sb.WriteString(d.text.Render(strings.Repeat(" ", ansi.StringWidth(count))))
			}
		}
	}
	return d.frame[t.level].Render(sb.String())
}

// countWidth is the width of the repeat count n after a space and the
// Times glyph, or zero for a toast seen once.
func (m Model) countWidth(n int) int {
	if n < 2 {
		return 0
	}
	return 1 + ansi.StringWidth(m.styles.Times) + len(strconv.Itoa(n))
}

func padRight(s string, width int) string {
	if w := ansi.StringWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

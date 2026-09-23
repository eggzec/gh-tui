package toast

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	// widthShare is the share of the width, in percent, the stack may take.
	widthShare = 40
	// minWidth keeps short messages readable in narrow layouts.
	minWidth = 24
	// maxLines is how many lines a message may wrap to before it is cut.
	maxLines = 3
	ellipsis = "…"
)

// View renders the stack, newest at the bottom, as a block whose toasts
// share one width and are aligned to its right edge. The block takes at most
// about 40% of the width and never more than the width. It is empty when
// there are no toasts.
func (m Model) View() string { return m.view }

// maxBlockWidth is the widest the stack may be.
func (m Model) maxBlockWidth() int {
	if m.width <= 0 {
		return minWidth
	}
	return min(max(m.width*widthShare/100, minWidth), m.width)
}

func (m Model) render() string {
	if len(m.toasts) == 0 {
		return ""
	}
	d := m.derived
	// The inner width holds the glyph, a space, the text and the count.
	maxInner := m.maxBlockWidth() - d.frameWidth
	inner := 0
	for _, t := range m.toasts {
		inner = max(inner, d.glyphWidth+1+ansi.StringWidth(t.text)+countWidth(t.count))
	}
	inner = min(inner, maxInner)
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
		count = " ×" + strconv.Itoa(t.count)
	}
	textWidth := inner - d.glyphWidth - 1 - ansi.StringWidth(count)
	if textWidth < 1 {
		// No room for the count; the text matters more.
		count = ""
		textWidth = inner - d.glyphWidth - 1
	}
	wrapped := strings.Split(ansi.Wrap(t.text, textWidth, ""), "\n")
	if len(wrapped) > maxLines {
		wrapped = wrapped[:maxLines]
		last := strings.TrimRight(wrapped[maxLines-1], " ")
		wrapped[maxLines-1] = ansi.Truncate(last+" "+ellipsis, textWidth, ellipsis)
	}

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

// countWidth is the width of " ×n", or zero for a toast seen once.
func countWidth(n int) int {
	if n < 2 {
		return 0
	}
	return 2 + len(strconv.Itoa(n))
}

func padRight(s string, width int) string {
	if w := ansi.StringWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

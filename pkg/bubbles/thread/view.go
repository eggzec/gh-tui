package thread

import "strings"

// View renders the lines on screen, exactly the width and height. It only
// slices lines rendered earlier, so scrolling never renders markdown or
// comments again.
func (m Model[T]) View() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	y := min(m.vp.YOffset(), len(m.lines))
	end := min(y+h, len(m.lines))

	size := (h - (end - y)) * (w + 1)
	for i := y; i < end; i++ {
		size += len(m.line(i)) + 1
	}
	// The head of the diagram the toggle key opens shows the pointer.
	pointed, marked := -1, ""
	if t := m.target(); m.focused && t != nil && t.marked != "" {
		pointed, marked = t.line, t.marked
	}
	var b strings.Builder
	b.Grow(size)
	for i := range h {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch {
		case y+i == pointed:
			b.WriteString(marked)
		case y+i < end:
			b.WriteString(m.line(y + i))
		default:
			b.WriteString(m.blank)
		}
	}
	return b.String()
}

// line returns line i, with the status kept current between layouts.
func (m *Model[T]) line(i int) string {
	if i == m.statusIdx {
		return m.status
	}
	return m.lines[i]
}

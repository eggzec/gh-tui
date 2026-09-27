package picker

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// View renders the picker at exactly its width and height: the input, a
// line with the scopes and the number of results, and the results below,
// inside the frame.
func (m Model) View() string { return m.view }

// inner returns the size inside the frame.
func (m Model) inner() (width, height int) {
	return m.width - m.styles.Frame.GetHorizontalFrameSize(),
		m.height - m.styles.Frame.GetVerticalFrameSize()
}

// listHeight returns the number of rows for results.
func (m Model) listHeight() int {
	_, h := m.inner()
	return max(h-2, 0)
}

// layout sizes the input to the room inside the frame, then renders.
func (m *Model) layout() {
	w, _ := m.inner()
	m.frame = newFrame(m.styles.Frame, w)
	m.lines = make([]string, len(m.rows))
	// The input draws one cell more than its width, for the cursor.
	m.input.SetWidth(max(w-ansi.StringWidth(m.prompt)-1, 1))
	// Scroll the input again for the new width.
	m.input.SetCursor(m.input.Position())
	m.scroll()
	m.render()
}

// render renders the view for the current state.
func (m *Model) render() {
	if m.width <= 0 || m.height <= 0 {
		m.view = ""
		return
	}
	w, h := m.inner()
	if w < 1 || h < 1 {
		// The frame alone doesn't fit.
		m.view = strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", m.width)+"\n", m.height), "\n")
		return
	}
	lines := make([]string, 0, h)
	lines = append(lines, fit(m.prompt+m.input.View(), w))
	if h > 1 {
		lines = append(lines, m.metaLine(w))
	}
	lines = m.appendList(lines, w, h-2)
	m.view = m.frame.wrap(lines)
}

// frame is what a frame draws around the lines inside it, rendered once
// for a width. Rendering the frame on every change costs more than the
// rows.
type frame struct {
	top, bottom []string
	left, right string
}

func newFrame(st lipgloss.Style, width int) frame {
	if width < 1 {
		return frame{}
	}
	// Escape sequences never hold an x, so it marks the content.
	lines := strings.Split(st.Render(strings.Repeat("x", width)), "\n")
	for i, l := range lines {
		left, rest, ok := strings.Cut(l, "x")
		if !ok {
			continue
		}
		right := rest[strings.LastIndexByte(rest, 'x')+1:]
		return frame{top: lines[:i], bottom: lines[i+1:], left: left, right: right}
	}
	return frame{}
}

func (f frame) wrap(lines []string) string {
	var b strings.Builder
	n := 0
	for _, l := range f.top {
		n += len(l) + 1
	}
	for _, l := range lines {
		n += len(f.left) + len(l) + len(f.right) + 1
	}
	for _, l := range f.bottom {
		n += len(l) + 1
	}
	b.Grow(n)
	for _, l := range f.top {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(f.left)
		b.WriteString(l)
		b.WriteString(f.right)
	}
	for _, l := range f.bottom {
		b.WriteByte('\n')
		b.WriteString(l)
	}
	return b.String()
}

// metaLine renders the scopes on the left and the state of the search on
// the right.
func (m Model) metaLine(w int) string {
	var left strings.Builder
	if len(m.scopes) > 0 {
		for i, name := range append([]string{"All"}, m.scopes...) {
			if i > 0 {
				left.WriteString("  ")
			}
			if i == m.scope {
				left.WriteString(m.styles.ActiveScope.Render(name))
			} else {
				left.WriteString(m.styles.Scope.Render(name))
			}
		}
	}
	var right string
	switch {
	case m.loading && len(m.results) > 0:
		right = m.spin.View()
	case m.err == nil && len(m.results) > 0:
		right = m.styles.Status.Render(count(len(m.results)))
	}
	rw := ansi.StringWidth(right)
	l := fit(left.String(), max(w-rw-1, 0))
	if rw > w {
		return fit(right, w)
	}
	return l + strings.Repeat(" ", w-ansi.StringWidth(l)-rw) + right
}

func count(n int) string {
	if n == 1 {
		return "1 result"
	}
	return strconv.Itoa(n) + " results"
}

// appendList appends n lines of results, or the state that stands in for
// them.
func (m *Model) appendList(lines []string, w, n int) []string {
	if n <= 0 {
		return lines
	}
	end := len(lines) + n
	switch {
	case m.err != nil:
		if text, hint := m.errorWords(m.err); text != "" {
			lines = append(lines, m.errorLine(text, hint, w))
		}
	case len(m.results) == 0 && m.loading:
		lines = append(lines, fit(m.spin.View()+m.styles.Empty.Render("Searching…"), w))
	case len(m.results) == 0:
		lines = append(lines, fit(m.styles.Empty.Render(m.emptyText), w))
	default:
		for i := m.top; i < len(m.rows) && len(lines) < end; i++ {
			if m.rows[i].item == m.sel {
				lines = append(lines, m.rowLine(m.rows[i], w))
				continue
			}
			if m.lines[i] == "" {
				m.lines[i] = m.rowLine(m.rows[i], w)
			}
			lines = append(lines, m.lines[i])
		}
	}
	blank := strings.Repeat(" ", w)
	for len(lines) < end {
		lines = append(lines, blank)
	}
	return lines
}

func (m Model) rowLine(r row, w int) string {
	if r.item < 0 {
		return fit(m.styles.Header.Render(r.header), w)
	}
	res := &m.results[r.item]
	gutter, title := "  ", m.styles.Title
	if r.item == m.sel {
		gutter, title = m.gutterOn, m.styles.SelectedTitle
	}
	var b strings.Builder
	b.WriteString(gutter)
	b.WriteString(highlight(res.title, res.matches, title, m.styles.Match))
	if res.detail != "" {
		b.WriteString("  ")
		b.WriteString(m.styles.Detail.Render(res.detail))
	}
	return fit(b.String(), w)
}

// highlight renders s in base, and the runes at the byte offsets in
// matches in match.
func highlight(s string, matches []int, base, match lipgloss.Style) string {
	if len(matches) == 0 {
		return base.Render(s)
	}
	var b strings.Builder
	done := 0
	for i := 0; i < len(matches); {
		start := matches[i]
		end := runeEnd(s, start)
		for i++; i < len(matches) && matches[i] == end; i++ {
			end = runeEnd(s, end)
		}
		if start > done {
			b.WriteString(base.Render(s[done:start]))
		}
		b.WriteString(match.Render(s[start:end]))
		done = end
	}
	if done < len(s) {
		b.WriteString(base.Render(s[done:]))
	}
	return b.String()
}

// runeEnd returns the offset after the rune at offset i of s.
func runeEnd(s string, i int) int {
	_, n := utf8.DecodeRuneInString(s[i:])
	return i + n
}

// fit truncates or pads styled text to exactly width cells.
// errorWords returns what the picker says of err, the failed search, and
// the hint after it.
func (m *Model) errorWords(err error) (text, hint string) {
	if m.errorText != nil {
		return m.errorText(err)
	}
	msg, _, _ := strings.Cut(err.Error(), "\n")
	return "Couldn't search: " + msg, ""
}

// errorLine renders the error row in w cells, the text cut to keep the
// hint whole.
func (m *Model) errorLine(text, hint string, w int) string {
	text = "✗ " + text
	if hint == "" {
		return fit(m.styles.Error.Render(text), w)
	}
	hint = " · " + hint
	if room := max(w-ansi.StringWidth(hint), 0); ansi.StringWidth(text) > room {
		text = ansi.Truncate(text, room, "…")
	}
	return fit(m.styles.Error.Render(text)+m.styles.Status.Render(hint), w)
}

func fit(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		s = ansi.Truncate(s, width, "…")
		w = ansi.StringWidth(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

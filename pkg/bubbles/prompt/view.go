package prompt

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View renders the prompt at exactly its width and height: the title, the
// input and the key hint, inside the frame. A prompt too short for all
// three drops the hint first, then the title.
func (m Model) View() string { return m.view }

// edges returns the left and right edge of each line for the current focus.
func (m Model) edges() edges {
	if m.focused {
		return m.focusedEdges
	}
	return m.blurredEdges
}

// edges are what a frame draws on either side of a line, rendered once.
type edges struct {
	left, right string
	width       int
}

// newEdges renders the left and right side of frame. Rendering the frame
// on every line costs more than the input itself.
func newEdges(frame lipgloss.Style) edges {
	frame = frame.UnsetBorderTop().UnsetBorderBottom().
		UnsetPaddingTop().UnsetPaddingBottom().UnsetMarginTop().UnsetMarginBottom()
	// Escape sequences never hold an x, so it marks the content.
	left, right, _ := strings.Cut(frame.Render("x"), "x")
	return edges{left: left, right: right, width: ansi.StringWidth(left) + ansi.StringWidth(right)}
}

// rows returns whether the title and hint fit, and how many rows the input
// gets.
func (m Model) rows() (title, hint bool, input int) {
	title, hint = m.height >= 2, m.height >= 3
	input = m.height
	if title {
		input--
	}
	if hint {
		input--
	}
	return title, hint, input
}

// layout sizes the input to the room inside the frame, then renders.
func (m *Model) layout() {
	inner := max(m.width-m.edges().width, 1)
	_, _, rows := m.rows()
	m.area.SetWidth(inner)
	m.area.SetHeight(max(rows, 1))
	// The input draws one cell more than its width, for the cursor.
	m.input.SetWidth(max(inner-1, 1))
	// Scroll the input again for the new width.
	m.input.SetCursor(m.input.Position())
	m.render()
}

func (m *Model) renderHint() {
	var b strings.Builder
	for _, k := range m.ShortHelp() {
		if !k.Enabled() || k.Help().Key == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(m.styles.Hint.Render(" · "))
		}
		b.WriteString(m.styles.Key.Render(k.Help().Key))
		b.WriteString(m.styles.Hint.Render(" " + k.Help().Desc))
	}
	m.hint = b.String()
}

// render renders the view for the current state.
func (m *Model) render() {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		m.view = ""
		return
	}
	e := m.edges()
	inner := max(w-e.width, 0)
	title, hint, rows := m.rows()

	lines := make([]string, 0, h)
	if title {
		lines = append(lines, fit(m.styles.Title.Render(clean(m.title)), inner))
	}
	var editor string
	if m.mode == SingleLine {
		editor = m.input.View()
	} else {
		editor = m.area.View()
	}
	ed := strings.Split(editor, "\n")
	for i := range rows {
		l := ""
		if i < len(ed) {
			l = ed[i]
		}
		lines = append(lines, fit(l, inner))
	}
	if hint {
		lines = append(lines, fit(m.hint, inner))
	}

	for i, l := range lines {
		l = e.left + l + e.right
		if inner == 0 {
			// The frame alone is wider than a very narrow prompt.
			l = fit(l, w)
		}
		lines[i] = l
	}
	m.view = strings.Join(lines, "\n")
}

// fit truncates or pads styled text to exactly width cells.
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

// clean puts text on one line without escape sequences, so it can't break
// the frame.
func clean(s string) string {
	return strings.Join(strings.Fields(termtext.OneLine(s)), " ")
}

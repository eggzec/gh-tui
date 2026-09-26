package cmdline

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// View renders the command line at exactly its width and [Model.Height]:
// the prompt and the line, scrolled so the cursor is in view.
func (m Model) View() string { return m.view }

// layout sizes the input to the room after the prompt, then renders.
func (m *Model) layout() {
	// The input draws one cell more than its width, for the cursor.
	if w := max(m.width-m.promptWidth-1, 1); w != m.input.Width() {
		m.input.SetWidth(w)
		// Scroll the input again for the new width. The input scrolls
		// only when the cursor leaves the part it shows, which a
		// narrower input may no longer reach, so the cursor goes to the
		// end and back. At the same width, the scroll stays as it is.
		pos := m.input.Position()
		m.input.CursorEnd()
		m.input.SetCursor(pos)
	}
	m.render()
}

// render renders the view for the current state.
func (m *Model) render() {
	if m.Height() == 0 {
		m.view = ""
		return
	}
	m.view = fit(m.promptView+m.inputView(), m.width)
}

// inputView returns the input's view. The input pads a placeholder with
// wide characters with NUL bytes, which draw nothing, so they go and fit
// pads the line instead.
func (m *Model) inputView() string {
	v := m.input.View()
	if m.input.Value() == "" && m.input.Placeholder != "" {
		v = strings.ReplaceAll(v, "\x00", "")
	}
	return v
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

// oneLine puts text on one line without escape sequences, keeping its
// spaces.
func oneLine(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(ansi.Strip(s))
}

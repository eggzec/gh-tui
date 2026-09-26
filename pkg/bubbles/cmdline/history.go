package cmdline

import (
	"slices"
	"strings"
)

// walk is a walk through the history with up and down, as in vim: it
// visits the lines that start with what was typed when it began.
type walk struct {
	active bool
	// prefix is the line as typed when the walk began.
	prefix string
	// cursor is where the cursor was, in runes, when the walk began.
	cursor int
	// at is the index in the history of the line shown, or its length
	// while the line is as typed.
	at int
}

// History returns a copy of the lines the command line recalls, oldest
// first.
func (m Model) History() []string { return slices.Clone(m.history) }

// SetHistory sets the lines the command line recalls, oldest first, such
// as the History the parent saved in an earlier session. The command line
// keeps a copy, up to its limit, and adds each line the user submits to it.
func (m *Model) SetHistory(lines []string) {
	if n := m.historyLimit; n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	m.history = slices.Clone(lines)
	m.walk = walk{}
}

// older shows the last line before the one shown that starts with the
// prefix, skipping lines like the one shown. It stays put when there is
// none.
func (m *Model) older() {
	if !m.walk.active {
		m.walk = walk{
			active: true,
			prefix: m.input.Value(),
			cursor: m.input.Position(),
			at:     len(m.history),
		}
	}
	for i := m.walk.at - 1; i >= 0; i-- {
		if m.recall(i) {
			return
		}
	}
}

// newer shows the next line after the one shown that starts with the
// prefix. Past the newest, it puts back the line as typed, with the cursor
// where it was, and ends the walk.
func (m *Model) newer() {
	if !m.walk.active {
		return
	}
	for i := m.walk.at + 1; i < len(m.history); i++ {
		if m.recall(i) {
			return
		}
	}
	typed := m.walk
	m.walk = walk{}
	m.input.SetValue(typed.prefix)
	m.input.SetCursor(typed.cursor)
	m.refresh()
}

// recall shows line i of the history if it starts with the prefix and
// differs from the line shown, and reports whether it did.
func (m *Model) recall(i int) bool {
	h := m.history[i]
	if !strings.HasPrefix(h, m.walk.prefix) || h == m.input.Value() {
		return false
	}
	m.walk.at = i
	m.show(h)
	return true
}

// show puts line in the input with the cursor after it.
func (m *Model) show(line string) {
	m.input.SetValue(line)
	m.input.CursorEnd()
	m.refresh()
}

// remember adds line to the end of the history, moving it there if the
// history has it already, and drops the oldest past the limit.
func (m *Model) remember(line string) {
	h := make([]string, 0, len(m.history)+1)
	for _, l := range m.history {
		if l != line {
			h = append(h, l)
		}
	}
	h = append(h, line)
	if n := m.historyLimit; n > 0 && len(h) > n {
		h = h[len(h)-n:]
	}
	m.history = h
	m.walk = walk{}
}

// Package confirm asks the user a yes or no question in a small box and
// takes the answer from a few keys.
package confirm

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// MaxLines is the most lines a question wraps to, so that a long one, such
// as a merge into a long branch, keeps its end at 80 columns.
const MaxLines = 2

// minWidth is the narrowest box that still shows a choice.
const minWidth = 14

// Ask is a change that waits for the user to say yes.
type Ask struct {
	// Question asks, such as "Close issue #12?".
	Question string
	// Run makes the change once the user says yes, and returns what sends
	// it.
	Run func() tea.Cmd
	// Default is the choice the box starts on, which enter then gives: true
	// for yes. Only a question that is safe and easy to undo defaults to yes;
	// the zero value, no, is for everything destructive, hard to undo or
	// bulk.
	Default bool
	// Danger draws the box in the error colour, for a change that is hard to
	// undo, such as a delete.
	Danger bool

	// switched is set while the user moved the choice off the default.
	switched bool
}

// Yes reports whether the choice is yes: the default, until the user
// switches it.
func (a Ask) Yes() bool { return a.Default != a.switched }

// Switch moves the choice to the other answer.
func (a *Ask) Switch() { a.switched = !a.switched }

// Height returns how many lines Box takes at width w: the borders, the
// question wrapped to at most [MaxLines] lines, and the choices.
func (a Ask) Height(w int) int {
	return 3 + len(a.wrap(max(w, minWidth)-4, "…"))
}

// wrap breaks the question into at most MaxLines lines of w cells; what
// doesn't fit, or a word longer than a line, such as a branch name, is cut.
func (a Ask) wrap(w int, ellipsis string) []string {
	// A question may name what GitHub calls something, such as a title.
	lines := wrapWords(termtext.OneLine(a.Question), w)
	if len(lines) > MaxLines {
		rest := strings.Join(lines[MaxLines-1:], " ")
		lines = append(lines[:MaxLines-1], termtext.Truncate(rest, w, ellipsis))
	}
	for i, l := range lines {
		lines[i] = termtext.Truncate(l, w, ellipsis)
	}
	return lines
}

// Box renders a as a box of w cells: a rounded border with Confirm on its
// top edge, the question, the choices, and the keys on the bottom edge. A
// destructive question is drawn in the error colour. It takes [Ask.Height]
// lines.
func (a Ask) Box(st Styles, k KeyMap, w int) []string {
	full := w
	w = max(w, minWidth)
	inner := w - 4
	border, title, sel := st.Border, st.Title, st.Selected
	if a.Danger {
		border, title, sel = st.Danger, st.DangerTitle, st.DangerSelected
	}
	lines := make([]string, 0, MaxLines+3)
	g := st.glyphs()
	lines = append(lines, edge(g.TopLeft, g.TopRight, g.Top, w, border, title.Render(" Confirm "), true))
	row := func(s string) {
		pad := strings.Repeat(" ", max(0, inner-ansi.StringWidth(s)))
		lines = append(lines, border.Render(g.Left)+" "+s+pad+" "+border.Render(g.Right))
	}
	for _, l := range a.wrap(inner, st.ellipsis()) {
		row(st.Question.Render(l))
	}
	row(choice("Yes", a.Yes(), sel, st.Keys) + "  " + choice("No", !a.Yes(), sel, st.Keys))
	hint := ""
	if h := a.hint(st, k); h != "" {
		hint = " " + st.Keys.Render(h) + " "
	}
	lines = append(lines, edge(g.BottomLeft, g.BottomRight, g.Bottom, w, border, hint, false))
	// Under its minimum the box is cut, so it never spills past w.
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, full, "")
	}
	return lines
}

// choice renders one answer: bracketed and in the accent while picked, and
// muted, with the same cells, while not.
func choice(label string, picked bool, sel, muted lipgloss.Style) string {
	if picked {
		return sel.Render("[ " + label + " ]")
	}
	return muted.Render("  " + label + "  ")
}

// edge builds a horizontal border of w cells holding item, which is the
// title against the left of the top edge, or the hint against the right of
// the bottom edge, and is dropped when it does not fit.
func edge(l, r, bar string, w int, border lipgloss.Style, item string, top bool) string {
	inner := w - 2
	if ansi.StringWidth(item)+2 > inner {
		item = ""
	}
	fill := max(0, inner-ansi.StringWidth(item))
	rule := func(n int) string { return border.Render(strings.Repeat(bar, n)) }
	switch {
	case item == "":
		return border.Render(l) + rule(inner) + border.Render(r)
	case top:
		return border.Render(l) + rule(1) + item + rule(fill-1) + border.Render(r)
	}
	return border.Render(l) + rule(fill-1) + item + rule(1) + border.Render(r)
}

// hint names the keys of the box, such as "y/n · tab: switch · enter:
// confirm", or the answers alone without the others.
func (a Ask) hint(st Styles, k KeyMap) string {
	parts := []string{}
	if s := k.answers(); s != "" {
		parts = append(parts, s)
	}
	for _, b := range []struct{ key, what string }{{k.Switch.Help().Key, "switch"}, {k.Default.Help().Key, "confirm"}} {
		if b.key == "" {
			continue
		}
		if st.Key != nil {
			b.key = st.Key(b.key)
		}
		parts = append(parts, b.key+": "+b.what)
	}
	return strings.Join(parts, st.separator())
}

// wrapWords wraps s into lines of at most w cells, breaking only at
// spaces, since ansi's wrapping breaks at hyphens too, which splits branch
// names. A word longer than w is left whole on a line of its own.
func wrapWords(s string, w int) []string {
	var lines []string
	var line string
	for word := range strings.FieldsSeq(s) {
		switch {
		case line == "":
			line = word
		case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	return append(lines, line)
}

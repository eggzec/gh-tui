// Package confirm asks the user a yes or no question on a line of a screen
// and takes the answer from a few keys.
package confirm

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// MaxLines is the most lines a question wraps to, so that a long one, such
// as a merge into a long branch, keeps its end at 80 columns.
const MaxLines = 2

// Ask is a change that waits for the user to say yes.
type Ask struct {
	// Question asks, such as "Close issue #12?".
	Question string
	// Run makes the change once the user says yes, and returns what sends
	// it.
	Run func() tea.Cmd
	// Default is the answer that enter gives: true for yes. Only a question
	// that is safe and easy to undo defaults to yes; the zero value, no, is
	// for everything destructive, hard to undo or bulk.
	Default bool
}

// Line renders a on a line of w cells: the question, cut to leave room, and
// the keys that answer it against the right edge.
func (a Ask) Line(st Styles, k KeyMap, w int) string {
	_, keys := a.keys(st, k)
	return termtext.Spread(st.Question.Render(termtext.OneLine(a.Question)), keys, w, st.ellipsis())
}

// Lines renders a on at most n lines of w cells: the question, wrapped at
// its spaces to leave room for the keys that answer it, which stand against
// the right edge of the last line. What doesn't fit in n lines, or a word
// longer than a line, such as a branch name, is cut.
func (a Ask) Lines(st Styles, k KeyMap, w, n int) []string {
	_, keys := a.keys(st, k)
	room := w - ansi.StringWidth(keys) - 1
	if n <= 1 || room < 1 {
		return []string{a.Line(st, k, w)}
	}
	// A question may name what GitHub calls something, such as a title.
	wrapped := wrapWords(termtext.OneLine(a.Question), room)
	if len(wrapped) > n {
		rest := strings.Join(wrapped[n-1:], " ")
		wrapped = append(wrapped[:n-1], termtext.Truncate(rest, room, st.ellipsis()))
	}
	lines := make([]string, len(wrapped))
	last := len(wrapped) - 1
	for i, l := range wrapped {
		l = st.Question.Render(l)
		if i < last {
			lines[i] = termtext.Fit(l, w, st.ellipsis())
		} else {
			lines[i] = termtext.Spread(l, keys, w, st.ellipsis())
		}
	}
	return lines
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

// keys renders what stands against the right edge: the answers, and, after
// the separator, the key that gives the default and what it answers, such
// as "y/n · enter: no". plain is the same without styles. Without a default
// key there is only the answers.
func (a Ask) keys(st Styles, k KeyMap) (plain, styled string) {
	plain, styled = k.answers(), st.Keys.Render(k.answers())
	dk := k.Default.Help().Key
	if dk == "" {
		return plain, styled
	}
	if st.Key != nil {
		dk = st.Key(dk)
	}
	answer := "no"
	if a.Default {
		answer = "yes"
	}
	sep := st.separator()
	plain += sep + dk + ": " + answer
	styled += st.Keys.Render(sep) + st.DefaultKey.Render(dk) + st.Keys.Render(": "+answer)
	return plain, styled
}

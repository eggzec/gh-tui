package pager

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// caseMode is how searches and filters match case: the zero value ignores
// it unless the pattern has a capital, as less's -i does.
type caseMode int

const (
	caseSmart caseMode = iota
	caseSensitive
	caseIgnore
)

// Notes on the options, shown in place of the name until the next key.
const (
	noteChop       = "Chop long lines"
	noteWrap       = "Wrap long lines"
	noteNumbers    = "Show line numbers"
	noteNoNumbers  = "Hide line numbers"
	noteSqueeze    = "Squeeze blank lines"
	noteNoSqueeze  = "Show all blank lines"
	noteSmartCase  = "Ignore case unless the pattern has capitals"
	noteIgnoreCase = "Ignore case in searches"
	noteMatchCase  = "Case is significant in searches"
	noteNoOption   = "No such option: -"
)

// updateOption toggles the option named by the key after the one bound to
// Option, as less's - does: by default S chops or wraps long lines, N
// shows or hides the line numbers, s squeezes runs of blank lines into
// one, and i and I set how the next search or filter matches case. The
// cancel key does nothing, and any other key says there is no such
// option.
func (m Model) updateOption(k tea.KeyPressMsg) (Model, tea.Cmd) {
	// The option keys are enabled only while one is awaited.
	o := m.keys.Options
	m.opt = false
	m.enableSearchKeys()
	var cmd tea.Cmd
	switch {
	case key.Matches(k, o.Chop):
		m.SetWrap(!m.wrap)
		m.info(either(m.wrap, noteWrap, noteChop))
	case key.Matches(k, o.LineNumbers):
		m.SetLineNumbers(!m.lineNumbers)
		m.info(either(m.lineNumbers, noteNumbers, noteNoNumbers))
	case key.Matches(k, o.Squeeze):
		p := m.want
		p.squeeze = !p.squeeze
		m.info(either(p.squeeze, noteSqueeze, noteNoSqueeze))
		cmd = m.project(p)
	case key.Matches(k, o.SmartCase):
		m.cases = either(m.cases == caseSmart, caseSensitive, caseSmart)
		m.info(m.cases.note())
	case key.Matches(k, o.IgnoreCase):
		m.cases = either(m.cases == caseIgnore, caseSensitive, caseIgnore)
		m.info(m.cases.note())
	case key.Matches(k, o.Cancel):
	default:
		m.flash = noteNoOption + termtext.OneLine(k.String())
	}
	return m, cmd
}

// note returns what the pager says of the case mode once it is set.
func (c caseMode) note() string {
	switch c {
	case caseSensitive:
		return noteMatchCase
	case caseIgnore:
		return noteIgnoreCase
	default:
		return noteSmartCase
	}
}

// info shows text in place of the name until the next key, as a note that
// isn't an error.
func (m *Model) info(text string) {
	m.flash, m.flashInfo = text, true
}

// either returns a if cond holds, and b otherwise.
func either[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

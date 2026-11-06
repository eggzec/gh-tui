package diff

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Notes on the options, shown in the status line until the next key.
const (
	noteChop      = "Chop long lines"
	noteWrap      = "Wrap long lines"
	noteNumbers   = "Show line numbers"
	noteNoNumbers = "Hide line numbers"
	noteNoOption  = "No such option: "
)

// OptionKeyMap holds the keys that name an option, after the view's Option
// key. The view enables them only while it waits for one.
type OptionKeyMap struct {
	// Chop chops or wraps long lines.
	Chop key.Binding `keymap:"chop" help:"chop or wrap long lines"`
	// LineNumbers shows or hides the line numbers.
	LineNumbers key.Binding `keymap:"line_numbers" help:"line numbers"`
	// Cancel chooses no option.
	Cancel key.Binding `keymap:"cancel" help:"cancel"`
}

// setEnabled enables or disables every option key.
func (o *OptionKeyMap) setEnabled(on bool) {
	for _, b := range []*key.Binding{&o.Chop, &o.LineNumbers, &o.Cancel} {
		keymap.Enable(b, on)
	}
}

// ChoosingOption reports whether the view waits for the name of an option,
// after the option key.
func (m Model) ChoosingOption() bool { return m.opt }

// updateOption toggles the option named by the key after the one bound to
// Option, as less's - does: by default S chops or wraps long lines and N shows or hides the line
// numbers.
// The cancel key does nothing, and any other key says there is no such
// option.
func (m *Model) updateOption(k tea.KeyPressMsg) {
	o := m.keyMap.Options
	m.opt = false
	m.syncKeys()
	switch {
	case keymap.Matches(k, o.Chop):
		m.SetWrap(!m.wrapOn)
		m.flash = either(m.wrapOn, noteWrap, noteChop)
	case keymap.Matches(k, o.LineNumbers):
		m.SetLineNumbers(!m.lineNumbers)
		m.flash = either(m.lineNumbers, noteNumbers, noteNoNumbers)
	case keymap.Matches(k, o.Cancel):
	default:
		m.flash = noteNoOption + termtext.OneLine(k.String())
	}
}

// SetLineNumbers shows or hides the line numbers. The rows of the blocks are
// rendered again, as their width changes with the gutter.
func (m *Model) SetLineNumbers(on bool) {
	if m.lineNumbers == on {
		return
	}
	m.lineNumbers = on
	m.setBlockWidth()
	m.left = min(m.left, m.maxLeft())
	m.scroll()
}

// LineNumbers reports whether the gutter shows the line numbers.
func (m Model) LineNumbers() bool { return m.lineNumbers }

// either returns a if cond holds, and b otherwise.
func either[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

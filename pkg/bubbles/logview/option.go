package logview

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Notes on the options, shown in place of the title until the next key.
const (
	noteChop      = "Chop long lines"
	noteWrap      = "Wrap long lines"
	noteNumbers   = "Show line numbers"
	noteNoNumbers = "Hide line numbers"
	noteNoTimes   = "Hide times"
	noteRelative  = "Show times since the section started"
	noteAbsolute  = "Show times of day"
	noteNoOption  = "No such option: -"
)

// OptionKeyMap holds the keys that name an option, after the view's
// Option key. The view enables them only while it waits for one.
type OptionKeyMap struct {
	// Chop chops or wraps long lines, and LineNumbers shows or hides the
	// line numbers. Timestamps shows the times relative to their section,
	// then the times of day, then hides them.
	Chop        key.Binding `keymap:"chop" help:"chop or wrap long lines"`
	LineNumbers key.Binding `keymap:"line_numbers" help:"line numbers"`
	Timestamps  key.Binding `keymap:"timestamps" help:"timestamps"`
	// Cancel chooses no option.
	Cancel key.Binding `keymap:"cancel" help:"cancel"`
}

// keysHelp lists the keys that name an option, as help words them: the
// ones that have a key.
func (o OptionKeyMap) keysHelp() string {
	var keys []string
	for _, b := range []key.Binding{o.Chop, o.LineNumbers, o.Timestamps} {
		if len(b.Keys()) > 0 {
			keys = append(keys, b.Help().Key)
		}
	}
	return strings.Join(keys, " ")
}

// setEnabled enables or disables every option key.
func (o *OptionKeyMap) setEnabled(on bool) {
	for _, b := range []*key.Binding{&o.Chop, &o.LineNumbers, &o.Timestamps, &o.Cancel} {
		b.SetEnabled(on)
	}
}

// ChoosingOption reports whether the view waits for the name of an
// option, after the option key.
func (m Model) ChoosingOption() bool { return m.opt }

// Searching reports whether the search input is open, which is what
// Capturing means unless the view waits for the name of an option.
func (m Model) Searching() bool { return m.searching }

// updateOption toggles the option named by the key after the one bound to
// Option, as less's - does: by default S chops or wraps long lines, N
// shows or hides the line numbers, and T shows the times, then the times
// of day, then hides them. The cancel key does nothing, and any other key
// says there is no such option.
func (m Model) updateOption(k tea.KeyPressMsg) (Model, tea.Cmd) {
	// The option keys are enabled only while one is awaited.
	o := m.keys.Options
	m.opt = false
	m.enableKeys()
	switch {
	case key.Matches(k, o.Chop):
		m.SetWrap(!m.wrap)
		m.info(either(m.wrap, noteWrap, noteChop))
	case key.Matches(k, o.LineNumbers):
		m.SetLineNumbers(!m.lineNumbers)
		m.info(either(m.lineNumbers, noteNumbers, noteNoNumbers))
	case key.Matches(k, o.Timestamps):
		m.SetTimeMode(m.times.next())
		m.info(m.times.note())
	case key.Matches(k, o.Cancel):
	default:
		m.flash = noteNoOption + termtext.OneLine(k.String())
	}
	return m, nil
}

// note returns what the view says of the time mode once it is set.
func (t TimeMode) note() string {
	switch t {
	case TimeRelative:
		return noteRelative
	case TimeAbsolute:
		return noteAbsolute
	default:
		return noteNoTimes
	}
}

// info shows text in place of the title until the next key, as a note that
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

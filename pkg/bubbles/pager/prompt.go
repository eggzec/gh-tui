package pager

import (
	"errors"
	"regexp"
	"regexp/syntax"
	"strings"
	"unicode"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
)

// Notes on a search, shown in place of the name until the next key.
const (
	noteNotFound = "Pattern not found"
	noteInvalid  = "Invalid pattern: "
)

// promptKeys returns the keys of the search prompt: only those of keys'
// CancelEmpty, which cancel it on an empty line. The pager takes Confirm
// and Cancel itself, and the prompt has no completion and no history.
func promptKeys(keys KeyMap) cmdline.KeyMap {
	return cmdline.NewKeyMap(keymap.Func(func(action string) []string {
		if action == "cancel_empty" {
			return keys.CancelEmpty.Keys()
		}
		return nil
	}))
}

// Prompts, which also say what the line typed after them is for.
const (
	promptSearch = "/"
	promptFilter = "&"
)

// openPrompt opens the prompt p.
func (m *Model) openPrompt(p string) tea.Cmd {
	m.prompt.SetPrompt(p)
	cmd := m.prompt.Open("")
	m.enableSearchKeys()
	return cmd
}

func (m *Model) closePrompt() {
	m.prompt.Blur()
	m.enableSearchKeys()
}

// updatePrompt passes msg to the open prompt. The pager submits and
// cancels the prompt itself, rather than waiting for the prompt's
// messages, so a key typed right after enter isn't lost to it.
func (m Model) updatePrompt(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(k, m.keys.Confirm):
			line, p := m.prompt.Value(), m.prompt.Prompt()
			m.closePrompt()
			if p == promptFilter {
				cmd := m.filterFor(line)
				return m, cmd
			}
			cmd := m.searchFor(line)
			return m, cmd
		case key.Matches(k, m.keys.Cancel):
			m.closePrompt()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	if !m.prompt.Focused() {
		// Backspace on an empty line closed it, and said so in cmd.
		m.closePrompt()
		return m, nil
	}
	return m, cmd
}

// searchFor runs the search the user typed after the prompt: a regexp
// that matches case as the options say, or, after a "!", the lines it
// doesn't match. A pattern that doesn't compile leaves the search shown
// as it was.
func (m *Model) searchFor(line string) tea.Cmd {
	pattern, invert := strings.CutPrefix(line, "!")
	if strings.TrimSpace(pattern) == "" {
		return nil
	}
	re, err := compile(pattern, m.cases)
	if err != nil {
		m.flash = noteInvalid + reason(err)
		return nil
	}
	return m.runSearch(line, re, invert, m.topLine(), false)
}

// compile compiles pattern to match case as c says.
func compile(pattern string, c caseMode) (*regexp.Regexp, error) {
	if c == caseIgnore || c == caseSmart && !strings.ContainsFunc(pattern, unicode.IsUpper) {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

// reason returns what is wrong with a pattern, without the pattern, which
// the user just typed.
func reason(err error) string {
	if se, ok := errors.AsType[*syntax.Error](err); ok {
		return string(se.Code)
	}
	return err.Error()
}

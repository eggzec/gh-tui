package prompt

import (
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

var (
	ctrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	bksp  = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

// testKeys returns the keys of a prompt, as the app sets them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	table := map[string][]string{
		"submit":      {"ctrl+s"},
		"submit_line": {"enter"},
		"cancel":      {"esc"},
	}
	return NewKeyMap(keytest.Table(table))
}

// newKeyed returns a prompt with the keys of the app.
func newKeyed(tb testing.TB, opts ...Option) Model {
	tb.Helper()
	return New(append([]Option{WithKeyMap(testKeys(tb))}, opts...)...)
}

// typeText types each rune of text as a key press.
func typeText(tb testing.TB, m Model, text string) Model {
	tb.Helper()
	for _, r := range text {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// send sends msgs in order and returns the message of the last command.
func send(tb testing.TB, m Model, msgs ...tea.Msg) (after Model, sent tea.Msg) {
	tb.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		m, cmd = m.Update(msg)
	}
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

// focused returns a focused prompt built with opts.
func focused(tb testing.TB, opts ...Option) Model {
	tb.Helper()
	m := newKeyed(tb, opts...)
	m.Focus()
	return m
}

func runeKey(s string) tea.KeyPressMsg {
	r, _ := utf8.DecodeRuneInString(s)
	return tea.KeyPressMsg{Code: r, Text: s}
}

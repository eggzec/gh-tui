package prompt

import (
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

var (
	ctrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	bksp  = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

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
	m := New(opts...)
	m.Focus()
	return m
}

func runeKey(s string) tea.KeyPressMsg {
	r, _ := utf8.DecodeRuneInString(s)
	return tea.KeyPressMsg{Code: r, Text: s}
}

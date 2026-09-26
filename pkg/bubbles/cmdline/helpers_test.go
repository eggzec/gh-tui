package cmdline

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	bksp  = tea.KeyPressMsg{Code: tea.KeyBackspace}
	left  = tea.KeyPressMsg{Code: tea.KeyLeft}
	home  = tea.KeyPressMsg{Code: tea.KeyHome}
)

// typeText types each rune of text as a key press.
func typeText(tb testing.TB, m Model, text string) Model {
	tb.Helper()
	for _, r := range text {
		m, _ = m.Update(runeKey(string(r)))
	}
	return m
}

// press sends msgs in order and returns the message of the last command.
func press(tb testing.TB, m Model, msgs ...tea.Msg) (after Model, sent tea.Msg) {
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

// opened returns a command line built with opts and opened with initial.
func opened(tb testing.TB, initial string, opts ...Option) Model {
	tb.Helper()
	m := New(opts...)
	m.Open(initial)
	return m
}

func runeKey(s string) tea.KeyPressMsg {
	r, _ := utf8.DecodeRuneInString(s)
	return tea.KeyPressMsg{Code: r, Text: s}
}

func assertFits(t *testing.T, v string, width, height int) {
	t.Helper()
	if height == 0 {
		if v != "" {
			t.Fatalf("view is %q, want empty", v)
		}
		return
	}
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, v)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

var (
	tab      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
)

// repos are the names the test completion offers.
var repos = []string{
	"gammons/slk", "gammons/slk-web", "cli/cli", "cli/go-gh",
	"charmbracelet/bubbletea", "charmbracelet/bubbles", "charmbracelet/lipgloss",
}

// completeWords completes the word before the cursor, after a space or a
// colon, with the words that start with it, replacing the word up to the
// cursor.
func completeWords(words ...string) Complete {
	return func(line string, cursor int) []Candidate {
		start := strings.LastIndexAny(line[:cursor], " :") + 1
		word := line[start:cursor]
		if word == "" {
			return nil
		}
		var out []Candidate
		for _, w := range words {
			if strings.HasPrefix(w, word) {
				out = append(out, Candidate{Text: w, Start: start, End: cursor})
			}
		}
		return out
	}
}

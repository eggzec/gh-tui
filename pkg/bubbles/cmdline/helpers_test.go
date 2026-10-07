package cmdline

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	bksp  = tea.KeyPressMsg{Code: tea.KeyBackspace}
	left  = tea.KeyPressMsg{Code: tea.KeyLeft}
	home  = tea.KeyPressMsg{Code: tea.KeyHome}
)

// testKeys returns the keys of the command line, as the app sets them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	table := map[string][]string{
		"run":           {"enter"},
		"cancel":        {"esc"},
		"cancel_empty":  {"backspace", "ctrl+h"},
		"complete":      {"tab"},
		"complete_prev": {"shift+tab"},
		"older":         {"up", "ctrl+p"},
		"newer":         {"down", "ctrl+n"},
	}
	return NewKeyMap(keytest.Table(table))
}

// newKeyed returns a command line with the keys of the app.
func newKeyed(tb testing.TB, historyLimit int, opts ...Option) Model {
	tb.Helper()
	return New(historyLimit, append([]Option{WithKeyMap(testKeys(tb))}, opts...)...)
}

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
// testHistoryLimit is how many lines the history of a test keeps.
const testHistoryLimit = 100

func opened(tb testing.TB, initial string, opts ...Option) Model {
	tb.Helper()
	m := newKeyed(tb, testHistoryLimit, opts...)
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

var (
	up   = tea.KeyPressMsg{Code: tea.KeyUp}
	down = tea.KeyPressMsg{Code: tea.KeyDown}
)

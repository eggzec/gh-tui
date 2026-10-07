package pager

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

const goSource = `package main

import "fmt"

// main greets the world, and then some: this comment is long enough to run past the edge of a narrow pager.
func main() {
	for i := range 3 {
		fmt.Println("hello", i) // 你好，世界
	}
}
`

var (
	escKey = tea.KeyPressMsg{Code: tea.KeyEscape}
	enter  = tea.KeyPressMsg{Code: tea.KeyEnter}
	space  = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
)

// press returns the key press that key.Matches reads as name.
func press(name string) tea.KeyPressMsg {
	switch name {
	case "esc":
		return escKey
	case "enter":
		return enter
	case "space":
		return space
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	if c, ok := strings.CutPrefix(name, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(name)
	return tea.KeyPressMsg{Code: r, Text: name}
}

// keys presses each key in order, as a program would: the messages that
// the pager sends itself, such as what a search found, go back to it. It returns
// the first other message of the last key's commands, if any.
func keys(tb testing.TB, m Model, names ...string) (after Model, sent tea.Msg) {
	tb.Helper()
	for _, name := range names {
		m, sent = deliver(m, press(name))
	}
	return m, sent
}

// deliver updates m with msg, and then with each message its commands send
// the pager itself, and returns the first message for anyone else.
func deliver(m Model, msg tea.Msg) (after Model, sent tea.Msg) {
	for msg != nil {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd == nil {
			return m, nil
		}
		if msg = cmd(); !isOwn(msg) {
			return m, msg
		}
	}
	return m, nil
}

// isOwn reports whether msg is one the pager sends itself.
func isOwn(msg tea.Msg) bool {
	switch msg.(type) {
	case searchMsg, projectMsg:
		return true
	}
	return false
}

// typeText types each rune of text as a key press.
func typeText(tb testing.TB, m Model, text string) Model {
	tb.Helper()
	for _, r := range text {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// open returns a focused pager showing text, highlighted when the name
// asks for it.
func open(tb testing.TB, name, text string, opts ...Option) Model {
	tb.Helper()
	m := New(append(defaults(tb), opts...)...)
	m.Focus()
	if cmd := m.SetContent(name, text); cmd != nil {
		m, _ = m.Update(cmd())
	}
	return m
}

// numbered returns n lines that say their number.
func numbered(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteString(" ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteByte('\n')
	}
	return b.String()
}

// plain returns the view without escape sequences.
func plain(m Model) string { return ansi.Strip(m.View()) }

func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, ansi.Strip(v))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

// previewKeys are the keys of the preview, which tests fill a pager's key
// map from.
var previewKeys = map[string][]string{
	"up": {"up", "k"}, "down": {"down", "j"}, "left": {"left", "h"}, "right": {"right", "l"},
	"page_up": {"b", "ctrl+b", "pgup"}, "page_down": {"space", "ctrl+f", "pgdown"},
	"half_page_up": {"ctrl+u"}, "half_page_down": {"ctrl+d"},
	"top": {"home", "g"}, "bottom": {"end", "G"},
	"option": {"-"}, "find": {"/"}, "quick_filter": {"&"},
	"next_match": {"n"}, "prev_match": {"N"}, "edit": {"v"},
	"global.quit": {"q"}, "global.dismiss": {"esc"},
	"search_prompt.run": {"enter"}, "search_prompt.cancel": {"esc"},
	"search_prompt.cancel_empty": {"backspace", "ctrl+h"},
	"pager_option.chop":          {"S"}, "pager_option.line_numbers": {"N"}, "pager_option.squeeze": {"s"},
	"pager_option.smart_case": {"i"}, "pager_option.ignore_case": {"I"}, "pager_option.cancel": {"esc"},
}

// lookup gives the keys of the preview.
var lookup = keytest.Table(previewKeys)

// testKeys returns the keys of a pager as the preview has them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	return NewKeyMap(lookup)
}

// defaults returns the options that give a pager the keys of the preview.
func defaults(tb testing.TB) []Option {
	tb.Helper()
	return []Option{WithKeyMap(testKeys(tb))}
}

// fresh returns a pager with the keys of the preview, and opts.
func fresh(tb testing.TB, opts ...Option) Model {
	tb.Helper()
	return New(append(defaults(tb), opts...)...)
}

// bg is a testing.TB for the setup that has no test, such as a benchmark's
// package-level fixture: it fails by panicking.
type bg struct{ testing.TB }

func (bg) Helper() {}

func (bg) Fatalf(format string, args ...any) { panic(fmt.Sprintf(format, args...)) }

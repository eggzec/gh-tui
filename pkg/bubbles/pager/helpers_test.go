package pager

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
	r, _ := utf8.DecodeRuneInString(name)
	return tea.KeyPressMsg{Code: r, Text: name}
}

// keys presses each key in order and returns the message of the last
// command, if any.
func keys(tb testing.TB, m Model, names ...string) (after Model, sent tea.Msg) {
	tb.Helper()
	var cmd tea.Cmd
	for _, name := range names {
		m, cmd = m.Update(press(name))
	}
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
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
	m := New(opts...)
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

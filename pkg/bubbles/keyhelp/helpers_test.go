package keyhelp

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// testKeys returns the keys of the help, as the app sets them.
func testKeys() KeyMap {
	table := map[string][]string{
		"up": {"up"}, "down": {"down"}, "page_up": {"pgup"}, "page_down": {"pgdown"},
		"top": {"home"}, "bottom": {"end"}, "capture": {"tab"}, "close": {"?"}, "cancel": {"esc"},
	}
	return NewKeyMap(func(action string) []string { return table[action] })
}

// newKeyed returns a help with the keys of the app.
func newKeyed(opts ...Option) Model {
	return New(append([]Option{WithKeyMap(testKeys())}, opts...)...)
}

var (
	esc      = tea.KeyPressMsg{Code: tea.KeyEscape}
	tab      = tea.KeyPressMsg{Code: tea.KeyTab}
	down     = tea.KeyPressMsg{Code: tea.KeyDown}
	up       = tea.KeyPressMsg{Code: tea.KeyUp}
	pgDown   = tea.KeyPressMsg{Code: tea.KeyPgDown}
	pgUp     = tea.KeyPressMsg{Code: tea.KeyPgUp}
	home     = tea.KeyPressMsg{Code: tea.KeyHome}
	end      = tea.KeyPressMsg{Code: tea.KeyEnd}
	ctrlR    = tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	question = tea.KeyPressMsg{Code: '?', Text: "?"}
)

// layers returns the keys of a pull request list inside an app: the app's
// keys first, then the list's, with a reload that the app's refresh
// shadows, a close that the list's own cancel wins esc from, and a merge
// that is off for now.
func layers() []Layer {
	merge := key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge"), key.WithDisabled())
	return []Layer{
		{Source: "app", Bindings: []key.Binding{
			bind("help", "?"),
			bind("quit", "ctrl+c"),
			bind("command", ":"),
			bind("refresh", "ctrl+r"),
		}},
		{Source: "pull requests", Bindings: []key.Binding{
			bind("down", "down", "j"),
			bind("up", "up", "k"),
			bind("open", "enter"),
			bind("reload", "r", "ctrl+r"),
			merge,
			bind("check out", "c"),
			bind("cancel", "esc"),
			bind("close", "q", "esc"),
		}},
	}
}

// open returns a focused help over layers.
func open(tb testing.TB, opts ...Option) Model {
	tb.Helper()
	m := newKeyed(append([]Option{WithLayers(layers()), WithTitle("Help · Pull requests"), WithSize(80, 24)}, opts...)...)
	m.Focus()
	return m
}

// press sends msgs in order, and returns the messages the commands they
// return send.
func press(tb testing.TB, m Model, msgs ...tea.Msg) (after Model, sent []tea.Msg) {
	tb.Helper()
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd == nil {
			continue
		}
		if c, ok := cmd().(CloseMsg); ok {
			sent = append(sent, c)
		}
	}
	return m, sent
}

// typeText types each rune of text.
func typeText(tb testing.TB, m Model, text string) Model {
	tb.Helper()
	for _, r := range text {
		m, _ = press(tb, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// descs returns the descriptions of the rows shown.
func descs(m Model) []string {
	shown := m.Shown()
	out := make([]string, 0, len(shown))
	for _, r := range shown {
		out = append(out, r.Binding.Help().Desc)
	}
	return out
}

func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	if width == 0 || height == 0 {
		if v != "" {
			tb.Errorf("%d×%d: view is %q, want empty", width, height, v)
		}
		return
	}
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Errorf("%d×%d: view has %d lines", width, height, len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("%d×%d: line %d is %d wide: %q", width, height, i, w, ansi.Strip(l))
		}
	}
}

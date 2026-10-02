package tui

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// arrowKeys is a key map whose help names keys with arrows, as the key
// maps of the bubbles do.
type arrowKeys struct{}

func (arrowKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open")),
	}
}
func (k arrowKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// bubbleGlyphs are drawn by bubbles whose styles don't take them yet: the
// separator of the status bar and the prompt of the help.
const bubbleGlyphs = "·›"

// nonASCII returns the runes of s, with its styles removed, that aren't
// ASCII, but for bubbleGlyphs.
func nonASCII(s string) string {
	var out []rune
	for _, r := range ansi.Strip(s) {
		if r > unicode.MaxASCII && !strings.ContainsRune(bubbleGlyphs, r) {
			out = append(out, r)
		}
	}
	return string(out)
}

// TestASCIIIconsDrawASCII checks that with ui.icons set to ascii the app
// draws nothing but ASCII: the header and its badge, the frames of the
// panes, the status bar with its hints, the rate limits and the
// connection, a modal's frame and tabs, and the help.
func TestASCIIIconsDrawASCII(t *testing.T) {
	s := core.RateStatus{Quotas: quotas(4812, 4960), Answered: statusAt.Add(-time.Minute), At: statusAt}
	m, fakes := newTestApp(t, WithRateStatus(&fixedRates{s: s}), WithLogin("laraibg786"), WithHost("github.com"))
	fakes[0].keyMap = arrowKeys{}
	fakes[3].badge = "3"
	m.Update(ui.SyncMsg{Key: "notifications"})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if got := nonASCII(m.View().Content); got == "" {
		t.Fatalf("the default icons draw only ASCII, so the test checks nothing")
	}
	runCommand(t, m, "set ui.icons="+config.IconsASCII)
	m.toast.Clear()
	v := m.View().Content
	if got := nonASCII(v); got != "" {
		t.Errorf("the repository screen draws %q:\n%s", got, ansi.Strip(v))
	}
	if bar := ansi.Strip(lastLine(m)); !strings.Contains(bar, "up/k up") || !strings.Contains(bar, "enter open") {
		t.Errorf("the hints name the arrow keys with glyphs: %q", bar)
	}

	drive(m, m.key(press("?")))
	if v := m.View().Content; nonASCII(v) != "" {
		t.Errorf("the help draws %q:\n%s", nonASCII(v), ansi.Strip(v))
	}
	drive(m, m.key(press("?")))

	run(m, ui.OpenModal(&tabbedModal{title: "Preview", names: []string{"Open", "Closed"}}))
	if v := m.View().Content; nonASCII(v) != "" {
		t.Errorf("a modal draws %q:\n%s", nonASCII(v), ansi.Strip(v))
	}
}

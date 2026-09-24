package history

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func testTheme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// testConfig reads nothing after a delay, so that tests run the reads of a
// rest at once.
func testConfig() config.History {
	h := config.Default().History
	h.Prefetch.HoverDelay = 0
	return h
}

// withClock fixes the clock and the time zone, so that ages and dates
// don't depend on when and where the tests run.
func withClock(now time.Time) Option {
	return func(o *options) {
		o.now = func() time.Time { return now }
		o.loc = time.UTC
	}
}

// baseNone is the base of the head of the default branch.
var baseNone = ui.BaseMsg{}

func testKeys() map[string][]string {
	return config.Default().Keys
}

// newModal returns the history of repo over f, of width by height, with
// main the default branch, loaded.
func newModal(tb testing.TB, f Service, width, height int, opts ...Option) (*Modal, *host) {
	tb.Helper()
	return newModalAt(tb, f, width, height, ui.BaseMsg{}, opts...)
}

func newModalAt(tb testing.TB, f Service, width, height int, base ui.BaseMsg, opts ...Option) (*Modal, *host) {
	tb.Helper()
	opts = append([]Option{WithConfig(testConfig()), withClock(testNow)}, opts...)
	m := New(tb.Context(), f, repo, "main", base, config.Default().Keys, opts...)
	m.SetTheme(testTheme())
	m.SetSize(width, height)
	h := &host{m: m}
	h.run(m.Init())
	return m, h
}

// host runs the commands of a modal the way the app would, feeding every
// message back to it, and keeps the messages meant for the app.
type host struct {
	m   *Modal
	got []tea.Msg
	// skip drops the messages it reports, such as rests, so a test can
	// look before they arrive.
	skip func(tea.Msg) bool
}

func isRest(msg tea.Msg) bool {
	_, ok := msg.(restMsg)
	return ok
}

// paneText is the text of pane p, without styles, its lines joined and
// its spaces collapsed, so that wrapped text reads whole.
func paneText(m *Modal, p pane) string {
	lines := m.paneLines(p, m.paneWidth(p), m.bodyHeight())
	return strings.Join(strings.Fields(ansi.Strip(strings.Join(lines, "\n"))), " ")
}

func (h *host) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if h.skip != nil && h.skip(msg) {
		return
	}
	switch msg := msg.(type) {
	case nil, spinner.TickMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
		return
	case ui.OpenMsg, ui.NotifyMsg, ui.CloseModalMsg, ui.ShowMsg, ui.BaseMsg:
		h.got = append(h.got, msg)
		return
	}
	if cmds, ok := sequence(msg); ok {
		for _, c := range cmds {
			h.run(c)
		}
		return
	}
	h.run(h.m.Update(msg))
}

// sequence unpacks the message of tea.Sequence, whose type is unexported.
func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i], _ = reflect.TypeAssert[tea.Cmd](v.Index(i))
	}
	return cmds, true
}

// keys presses each key and runs what it returns.
func (h *host) keys(ks ...string) {
	for _, k := range ks {
		h.run(h.m.Update(press(k)))
	}
}

// take returns the messages for the app, and forgets them.
func (h *host) take() []tea.Msg {
	out := h.got
	h.got = nil
	return out
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// screen returns the view without styles.
func screen(m *Modal) string {
	return ansi.Strip(m.View())
}

// assertFits checks that v is exactly height lines of exactly width cells.
func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Errorf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

package actions

import (
	"log/slog"
	"os"
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

// TestMain drops what the modal logs, which would otherwise go to the
// test output and garble benchmark results.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

// The sizes inside the frame on terminals of 190 by 50 and 80 by 24.
const (
	wideW, wideH     = 148, 38
	narrowW, narrowH = 60, 18
)

func testTheme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// forTests fixes the clock, reads at once rather than after a rest, reads
// nothing ahead, and stops the timers, so that tests don't wait and see
// only the reads of what the panes show.
func forTests() Option {
	return func(o *options) {
		o.now = func() time.Time { return testNow }
		o.prefetch = testPrefetch(func(p *config.PrefetchLayers) { p.Enabled = false })
		o.tick = 0
	}
}

// testPrefetch returns the modal's prefetch settings as the defaults have
// them, but with no rest, so that tests run the reads of a rest at once,
// after edit changes the settings.
func testPrefetch(edit func(p *config.PrefetchLayers)) prefetch {
	p := config.Default().Prefetch
	p.Actions.Rest = new(time.Duration(0))
	edit(&p)
	return newPrefetch(p)
}

// withPrefetch reads ahead as the defaults do, after edit, if set, changes
// them, with no rest.
func withPrefetch(edit func(p *config.PrefetchLayers)) Option {
	return func(o *options) {
		o.prefetch = testPrefetch(func(p *config.PrefetchLayers) {
			if edit != nil {
				edit(p)
			}
		})
	}
}

// withIcons sets the unicode glyphs, which read in goldens.
func withIcons() Option {
	return WithIcons(ui.NewIcons(config.IconsUnicode))
}

// testKeys are the default keys, with E to re-run every job of a run,
// which has no key by default.
func testKeys() config.Keymap {
	keys := config.Default().Keys
	keys.Set("actions.rerun", []string{"E"})
	return keys
}

// newModal returns the modal of repo over f, of width by height, loaded.
func newModal(tb testing.TB, f Service, width, height int, opts ...Option) (*Modal, *host) {
	tb.Helper()
	return newModalKeys(tb, testKeys(), f, width, height, opts...)
}

// newModalKeys is newModal with the keys given.
func newModalKeys(tb testing.TB, keys config.Keymap, f Service, width, height int, opts ...Option) (*Modal, *host) {
	tb.Helper()
	opts = append([]Option{forTests(), withIcons(), WithViewer(viewer)}, opts...)
	m := New(tb.Context(), f, repo, keys, opts...)
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
	// hold keeps the messages it reports for later, such as the answer to
	// a change, so a test can look before they arrive.
	hold func(tea.Msg) bool
	held []tea.Msg
}

func (h *host) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if h.hold != nil && h.hold(msg) {
		h.held = append(h.held, msg)
		return
	}
	h.handle(msg)
}

// handle takes msg as the app would.
func (h *host) handle(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil, spinner.TickMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
		return
	case ui.OpenMsg, ui.NotifyMsg, ui.CloseModalMsg, ui.OpenFileMsg:
		h.got = append(h.got, msg)
		return
	case ui.DoneMsg:
		// The app shows the error, then passes the message on.
		h.got = append(h.got, msg)
	}
	if cmds, ok := sequence(msg); ok {
		for _, c := range cmds {
			h.run(c)
		}
		return
	}
	h.run(h.m.Update(msg))
}

// release delivers the held messages. The messages they lead to are held
// in turn, until the test clears hold.
func (h *host) release() {
	held := h.held
	h.held = nil
	for _, msg := range held {
		h.handle(msg)
	}
}

// send delivers msg as the app would.
func (h *host) send(msg tea.Msg) {
	h.run(func() tea.Msg { return msg })
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
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
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
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	}
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// screen returns the view without styles.
func screen(m *Modal) string {
	return ansi.Strip(m.View())
}

// paneText is the text of pane p, without styles, its spaces collapsed.
func paneText(m *Modal, p pane) string {
	lines := m.paneLines(p, m.paneWidth(p), m.bodyHeight())
	return strings.Join(strings.Fields(ansi.Strip(strings.Join(lines, "\n"))), " ")
}

// lastLine is the last line of the view, without styles, trimmed.
func lastLine(m *Modal) string {
	lines := strings.Split(screen(m), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
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

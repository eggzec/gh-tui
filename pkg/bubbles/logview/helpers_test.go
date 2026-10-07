package logview

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// markers are the prefixes the runner puts on lines, and their kinds.
var markers = []struct {
	prefix string
	kind   Kind
}{
	{"##[group]", Group},
	{"##[endgroup]", EndGroup},
	{"##[error]", Error},
	{"##[warning]", Warning},
	{"##[notice]", Notice},
	{"##[debug]", Debug},
	{"[command]", Command},
}

// parse parses a raw GitHub Actions job log the way the data layer does:
// a timestamp, then a marker, then the text.
func parse(raw string) []Line {
	raw = strings.TrimPrefix(raw, "\ufeff")
	var out []Line
	for l := range strings.SplitSeq(strings.TrimSuffix(raw, "\n"), "\n") {
		l = strings.TrimSuffix(l, "\r")
		var t time.Time
		if ts, rest, ok := strings.Cut(l, " "); ok {
			if tt, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				t, l = tt, rest
			}
		}
		kind := Plain
		for _, mk := range markers {
			if rest, ok := strings.CutPrefix(l, mk.prefix); ok {
				kind, l = mk.kind, rest
				break
			}
		}
		out = append(out, Line{Time: t, Text: l, Kind: kind})
	}
	return out
}

// step is a step of the fixture's job, as the jobs API lists it, and the
// text of the line it starts at.
type step struct {
	name, first string
	failed      bool
	took        time.Duration
}

const checkout = "Run actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"

// lintSteps are the steps of the job in testdata/lint.txt. Skipped steps
// have no lines, and start where the next one does.
var lintSteps = []step{
	{name: "Set up job", first: "Current runner version: '2.337.0'", took: time.Second},
	{name: "Run git config --global core.autocrlf input", first: "Run git config --global core.autocrlf input", took: time.Second},
	{name: checkout, first: checkout, took: 5 * time.Second},
	{name: checkout, first: checkout, took: 3 * time.Second},
	{name: "Install Go", first: "Run actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e", took: 22 * time.Second},
	{name: "Pre-run command"},
	{name: "golangci-lint", first: "Run golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a",
		failed: true, took: 32 * time.Second},
	{name: "Post golangci-lint", first: "Post job cleanup.", took: 2 * time.Second},
	{name: "Post Install Go"},
	{name: "Post " + checkout, first: "Post job cleanup.", took: 2 * time.Second},
	{name: "Post " + checkout, first: "Post job cleanup.", took: 2 * time.Second},
	{name: "Complete job", first: "Cleaning up orphan processes"},
}

// sections returns the sections of steps in lines: each starts at the
// next line with its first text, and ends where the next one starts.
func sections(lines []Line, steps []step) []Section {
	out := make([]Section, len(steps))
	i := 0
	for s, st := range steps {
		if st.first != "" {
			for i < len(lines) && lines[i].Text != st.first {
				i++
			}
		}
		out[s] = Section{Title: st.name, Start: i, Failed: st.failed, Duration: st.took}
		if st.first != "" && i < len(lines) {
			i++
		}
	}
	end := len(lines)
	for s := len(out) - 1; s >= 0; s-- {
		if steps[s].first == "" {
			// A skipped step is empty, where the next one starts.
			out[s].Start = end
		}
		out[s].End = end
		end = out[s].Start
	}
	return out
}

// fixture returns the lines and sections of testdata/lint.txt, a trimmed
// log of a golangci-lint job of charmbracelet/bubbletea that failed.
var fixture = sync.OnceValues(func() ([]Line, []Section) {
	raw, err := os.ReadFile("testdata/lint.txt")
	if err != nil {
		panic(err)
	}
	lines := parse(string(raw))
	return lines, sections(lines, lintSteps)
})

// open returns a focused view of the fixture.
func open(tb testing.TB, opts ...Option) Model {
	tb.Helper()
	lines, secs := fixture()
	m := New(append([]Option{WithKeyMap(testKeys(tb))}, opts...)...)
	m.Focus()
	m.SetTitle("lint / lint (windows-latest)")
	m.SetLines(lines, secs)
	return m
}

// view returns a focused view of lines without sections.
func view(tb testing.TB, lines []Line, opts ...Option) Model {
	tb.Helper()
	m := New(append([]Option{WithKeyMap(testKeys(tb))}, opts...)...)
	m.Focus()
	m.SetLines(lines, nil)
	return m
}

// plainLines returns lines of kind Plain with the given texts.
func plainLines(texts ...string) []Line {
	out := make([]Line, len(texts))
	for i, t := range texts {
		out[i] = Line{Text: t}
	}
	return out
}

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
	if c, ok := strings.CutPrefix(name, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
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

// find searches for query as a user would.
func find(tb testing.TB, m Model, query string) Model {
	tb.Helper()
	m, _ = keys(tb, m, "/")
	m = typeText(tb, m, query)
	m, _ = keys(tb, m, "enter")
	return m
}

// cursorText returns the text of the row under the cursor.
func cursorText(m Model) string {
	if r := m.cursorRow(); r >= 0 {
		return m.rows[r].text
	}
	return ""
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

// actionsLogKeys are the keys of the Actions log, which tests fill a log
// view's key map from.
var actionsLogKeys = map[string][]string{
	"up": {"up", "k"}, "down": {"down", "j"}, "left": {"left", "h"}, "right": {"right", "l"},
	"page_up": {"b", "ctrl+b", "pgup"}, "page_down": {"space", "ctrl+f", "pgdown"},
	"half_page_up": {"ctrl+u"}, "half_page_down": {"ctrl+d"},
	"top": {"home", "g"}, "bottom": {"end", "G"},
	"expand": {"+"}, "collapse": {"-"}, "expand_all": {"*"},
	"next_error": {"e"}, "prev_error": {"E"}, "next_warning": {"w"}, "prev_warning": {"W"},
	"wrap": {"s"}, "times": {"t"}, "line_numbers": {"#"}, "follow": {"F"},
	"find": {"/"}, "next_match": {"n"}, "prev_match": {"N"},
	"global.select": {"enter"}, "global.quit": {"q"}, "global.dismiss": {"esc"},
	"search_prompt.run": {"enter"}, "search_prompt.cancel": {"esc"},
}

// lookup gives the keys of the Actions log.
var lookup = keytest.Table(actionsLogKeys)

// testKeys returns the keys of a log view as the Actions log has them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	return NewKeyMap(lookup)
}

// withKeys gives a log view the keys of the Actions log.
func withKeys(tb testing.TB) Option {
	tb.Helper()
	return WithKeyMap(testKeys(tb))
}

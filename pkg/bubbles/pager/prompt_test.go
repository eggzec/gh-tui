package pager

import (
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// enterAll presses the keys named esc, enter and backspace, and types
// every other part rune by rune. It returns the message of the last key.
func enterAll(tb testing.TB, m Model, parts ...string) (after Model, sent tea.Msg) {
	tb.Helper()
	for _, p := range parts {
		if slices.Contains([]string{"esc", "enter", "backspace"}, p) {
			m, sent = keys(tb, m, p)
			continue
		}
		for _, r := range p {
			m, sent = keys(tb, m, string(r))
		}
	}
	return m, sent
}

func TestPrompt(t *testing.T) {
	// "line x" is on every line but every seventh from the first, and
	// "line x " on every seventh from the second.
	text := numbered(100)
	tests := []struct {
		name          string
		keys          []string
		wantQuery     string
		wantMatches   int
		wantLine      int
		wantNote      string
		wantCapturing bool
		wantClose     bool
	}{
		{name: "slash opens the prompt", keys: []string{"/"}, wantLine: -1, wantCapturing: true},
		{name: "enter searches", keys: []string{"/", "line x ", "enter"},
			wantQuery: "line x ", wantMatches: 15, wantLine: 1},
		{name: "n goes to the next match", keys: []string{"/", "line x ", "enter", "n"},
			wantQuery: "line x ", wantMatches: 15, wantLine: 8},
		{name: "N goes back, around the start", keys: []string{"/", "line x ", "enter", "n", "N", "N"},
			wantQuery: "line x ", wantMatches: 15, wantLine: 99},
		{name: "patterns are regexps", keys: []string{"/", "l.ne xx ", "enter"},
			wantQuery: "l.ne xx ", wantMatches: 14, wantLine: 2},
		{name: "capitals match case", keys: []string{"/", "LINE", "enter"},
			wantLine: -1, wantNote: "Pattern not found"},
		{name: "bang finds the lines that don't match", keys: []string{"/", "!line x", "enter"},
			wantQuery: "!line x", wantMatches: 15, wantLine: 0},
		{name: "n steps through the lines that don't match", keys: []string{"/", "!line x", "enter", "n"},
			wantQuery: "!line x", wantMatches: 15, wantLine: 7},
		{name: "an invalid pattern keeps the search", keys: []string{"/", "line x ", "enter", "n", "/", "(", "enter"},
			wantQuery: "line x ", wantMatches: 15, wantLine: 8, wantNote: "Invalid pattern: missing closing )"},
		{name: "not found clears the search", keys: []string{"/", "line x ", "enter", "/", "kiwi", "enter"},
			wantLine: -1, wantNote: "Pattern not found"},
		{name: "the note goes at the next key", keys: []string{"/", "kiwi", "enter", "j"}, wantLine: -1},
		{name: "enter on an empty line keeps the search", keys: []string{"/", "line x ", "enter", "/", "enter"},
			wantQuery: "line x ", wantMatches: 15, wantLine: 1},
		{name: "esc closes the prompt first", keys: []string{"/", "line x ", "enter", "/", "ab", "esc"},
			wantQuery: "line x ", wantMatches: 15, wantLine: 1},
		{name: "then clears the search", keys: []string{"/", "line x ", "enter", "/", "esc", "esc"}, wantLine: -1},
		{name: "then closes", keys: []string{"/", "line x ", "enter", "/", "esc", "esc", "esc"},
			wantLine: -1, wantClose: true},
		{name: "backspace edits the line", keys: []string{"/", "a", "backspace"}, wantLine: -1, wantCapturing: true},
		{name: "backspace on an empty line cancels", keys: []string{"/", "a", "backspace", "backspace"}, wantLine: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "lines.txt", text, WithSize(80, 11))
			m, msg := enterAll(t, m, tt.keys...)
			if m.Query() != tt.wantQuery || m.Matches() != tt.wantMatches {
				t.Errorf("query %q with %d matches, want %q with %d", m.Query(), m.Matches(), tt.wantQuery, tt.wantMatches)
			}
			line := -1
			if m.search.cur >= 0 {
				line = m.search.curLine
			}
			if line != tt.wantLine {
				t.Errorf("current match on line %d, want %d", line, tt.wantLine)
			}
			if m.flash != tt.wantNote {
				t.Errorf("note %q, want %q", m.flash, tt.wantNote)
			}
			if tt.wantNote != "" && !strings.HasPrefix(lastLine(plain(m)), tt.wantNote) {
				t.Errorf("status %q doesn't start with the note", lastLine(plain(m)))
			}
			if m.Capturing() != tt.wantCapturing {
				t.Errorf("capturing %v, want %v", m.Capturing(), tt.wantCapturing)
			}
			if tt.wantCapturing && !strings.HasPrefix(lastLine(plain(m)), "/") {
				t.Errorf("status %q isn't the prompt", lastLine(plain(m)))
			}
			if _, ok := msg.(CloseMsg); ok != tt.wantClose {
				t.Errorf("sent %#v, want close %v", msg, tt.wantClose)
			}
		})
	}
}

// An inverted search marks the lines it matches in the gutter, with the
// line numbers or, while they are hidden, in a cell of its own.
func TestInvertedMarks(t *testing.T) {
	for _, numbers := range []bool{true, false} {
		m := open(t, "a.txt", "a\nb\na\nb\n", WithSize(20, 5), WithLineNumbers(numbers))
		width := m.gutterWidth()
		m, _ = enterAll(t, m, "/", "!a", "enter")
		if m.Matches() != 2 || m.search.curLine != 1 {
			t.Fatalf("numbers %v: %d matches, current on line %d; want 2, on line 1", numbers, m.Matches(), m.search.curLine)
		}
		if _, _, hit := m.lineHits(3); !hit {
			t.Errorf("numbers %v: line 3 isn't marked", numbers)
		}
		if _, _, hit := m.lineHits(0); hit {
			t.Errorf("numbers %v: line 0 is marked", numbers)
		}
		if got := m.gutterWidth(); got != max(width, 2) {
			t.Errorf("numbers %v: gutter %d wide, want %d", numbers, got, max(width, 2))
		}
		m, _ = keys(t, m, "esc")
		if got := m.gutterWidth(); got != width {
			t.Errorf("numbers %v: gutter %d wide after esc, want %d", numbers, got, width)
		}
	}
}

// Only the keys of search_prompt.cancel_empty close the prompt on an empty
// line: with ctrl+h alone, backspace there leaves it open.
func TestPromptCancelEmptyFromKeys(t *testing.T) {
	table := maps.Clone(previewKeys)
	table["search_prompt.cancel_empty"] = []string{"ctrl+h"}
	m := open(t, "lines.txt", numbered(10), WithKeyMap(NewKeyMap(keytest.Table(table))))
	m, _ = keys(t, m, "/", "backspace")
	if !m.Capturing() {
		t.Fatal("backspace on an empty line closed the prompt, which only ctrl+h should")
	}
	m, _ = keys(t, m, "ctrl+h")
	if m.Capturing() {
		t.Error("ctrl+h on an empty line left the prompt open")
	}
}

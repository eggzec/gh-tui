package pager

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"
)

func TestScroll(t *testing.T) {
	// 100 lines in a window of 10 rows and the status line.
	tests := []struct {
		name    string
		keys    []string
		wantTop int
	}{
		{name: "down", keys: []string{"j"}, wantTop: 1},
		{name: "down arrow", keys: []string{"down", "down"}, wantTop: 2},
		{name: "up at the top", keys: []string{"k", "up"}, wantTop: 0},
		{name: "up", keys: []string{"j", "j", "k"}, wantTop: 1},
		{name: "page down", keys: []string{"f"}, wantTop: 10},
		{name: "space", keys: []string{"space", "space"}, wantTop: 20},
		{name: "pgdown", keys: []string{"pgdown"}, wantTop: 10},
		{name: "page up", keys: []string{"f", "f", "b"}, wantTop: 10},
		{name: "pgup", keys: []string{"f", "pgup"}, wantTop: 0},
		{name: "half page down", keys: []string{"d"}, wantTop: 5},
		{name: "half page up", keys: []string{"d", "d", "u"}, wantTop: 5},
		{name: "end", keys: []string{"G"}, wantTop: 90},
		{name: "end key", keys: []string{"end"}, wantTop: 90},
		{name: "home", keys: []string{"G", "g"}, wantTop: 0},
		{name: "home key", keys: []string{"f", "home"}, wantTop: 0},
		{name: "down stops at the end", keys: slices.Repeat([]string{"f"}, 20), wantTop: 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
			m, _ = keys(t, m, tt.keys...)
			if m.top != tt.wantTop {
				t.Errorf("top = %d, want %d", m.top, tt.wantTop)
			}
		})
	}
}

func TestScrollShortContent(t *testing.T) {
	m := open(t, "lines.txt", numbered(3), WithSize(40, 11))
	for _, k := range []string{"j", "f", "G", "d"} {
		if m, _ = keys(t, m, k); m.top != 0 {
			t.Errorf("after %s top = %d, want 0", k, m.top)
		}
	}
}

func TestScrollWrapped(t *testing.T) {
	// Every line takes three rows of 10 columns.
	text := strings.Repeat(strings.Repeat("abcdefghij", 3)+"\n", 10)
	m := open(t, "a.txt", text, WithSize(10, 5), WithLineNumbers(false), WithWrap(true))
	tests := []struct {
		keys             []string
		wantTop, wantRow int
	}{
		{keys: []string{"j"}, wantTop: 0, wantRow: 1},
		{keys: []string{"j", "j", "j"}, wantTop: 1, wantRow: 0},
		{keys: []string{"j", "j", "j", "k"}, wantTop: 0, wantRow: 2},
		{keys: []string{"f"}, wantTop: 1, wantRow: 1},
		// 30 rows and a window of 4 end at the second row of line 9.
		{keys: []string{"G"}, wantTop: 8, wantRow: 2},
		{keys: []string{"G", "f"}, wantTop: 8, wantRow: 2},
		{keys: []string{"G", "g"}, wantTop: 0, wantRow: 0},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, " "), func(t *testing.T) {
			got, _ := keys(t, m, tt.keys...)
			if got.top != tt.wantTop || got.row != tt.wantRow {
				t.Errorf("at %d:%d, want %d:%d", got.top, got.row, tt.wantTop, tt.wantRow)
			}
		})
	}
}

func TestScrollSideways(t *testing.T) {
	long := strings.Repeat("0123456789", 5) // 50 columns
	m := open(t, "a.txt", long+"\nshort\n", WithSize(20, 4), WithLineNumbers(false))
	m, _ = keys(t, m, "l")
	if m.left != 5 {
		t.Errorf("left = %d, want 5", m.left)
	}
	m, _ = keys(t, m, "right", "l", "l", "l", "l", "l", "l", "l")
	if m.left != 30 {
		t.Errorf("left = %d, want 30: the long line ends at the edge", m.left)
	}
	m, _ = keys(t, m, "h", "left")
	if m.left != 20 {
		t.Errorf("left = %d, want 20", m.left)
	}
	m, _ = keys(t, m, "-", "S")
	if m.left != 0 || !m.Wrap() {
		t.Errorf("wrap = %v, left = %d; want wrapped from column 0", m.Wrap(), m.left)
	}
	m, _ = keys(t, m, "l")
	if m.left != 0 {
		t.Errorf("left = %d while wrapped, want 0", m.left)
	}
}

func TestSearch(t *testing.T) {
	text := "Apple pie\nbanana\napple and apple\ncherry\n"
	tests := []struct {
		name        string
		query       string
		wantMatches int
		wantCur     int
		wantTop     int
	}{
		{name: "lower case ignores case", query: "apple", wantMatches: 3},
		{name: "capitals match case", query: "Apple", wantMatches: 1},
		{name: "patterns are regexps", query: "a.d", wantMatches: 1, wantTop: 2},
		// The window of two rows ends at the last line.
		{name: "jumps down to the match", query: "cherry", wantMatches: 1, wantTop: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "fruit.txt", text, WithSize(20, 3))
			m, _ = keys(t, m, "/")
			m = typeText(t, m, tt.query)
			m, _ = keys(t, m, "enter")
			if m.Capturing() {
				t.Error("still capturing after enter")
			}
			if m.Query() != tt.query || m.Matches() != tt.wantMatches {
				t.Errorf("query %q with %d matches, want %q with %d",
					m.Query(), m.Matches(), tt.query, tt.wantMatches)
			}
			if m.search.cur != tt.wantCur || m.top != tt.wantTop {
				t.Errorf("match %d at top %d, want %d at %d", m.search.cur, m.top, tt.wantCur, tt.wantTop)
			}
			if got := m.KeyMap().Next.Enabled(); got != (tt.wantMatches > 0) {
				t.Errorf("next enabled = %v with %d matches", got, tt.wantMatches)
			}
		})
	}
}

func TestSetSearch(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
	m, _ = keys(t, m, "G")
	m.SetSearch("line x ")
	if m.Capturing() || m.Query() != "line x " || m.Matches() != 15 {
		t.Fatalf("query %q with %d matches, capturing %v; want 15 of %q", m.Query(), m.Matches(), m.Capturing(), "line x ")
	}
	// The first match, on line 2, not the first after the window.
	if m.search.cur != 0 || m.top != 0 {
		t.Errorf("match %d at top %d, want the first at the top", m.search.cur, m.top)
	}
	if m, _ = keys(t, m, "n"); m.search.cur != 1 {
		t.Errorf("n went to match %d, want 1", m.search.cur)
	}
	m.SetSearch("")
	if m.Query() != "" || m.KeyMap().Next.Enabled() {
		t.Error("an empty query should clear the search")
	}
	m.SetContent("other.txt", "text")
	if m.Query() != "" {
		t.Error("new content should clear the search")
	}
}

func TestSearchSteps(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
	m, _ = keys(t, m, "/")
	m = typeText(t, m, "line x ")
	m, _ = keys(t, m, "enter")
	// "line x " is on lines 2, 9, 16, …, 100: 15 of them.
	if m.Matches() != 15 {
		t.Fatalf("%d matches, want 15", m.Matches())
	}
	steps := []struct {
		key              string
		wantCur, wantTop int
	}{
		// Line 16 is in the window already, so it doesn't scroll.
		{key: "n", wantCur: 1, wantTop: 0},
		{key: "n", wantCur: 2, wantTop: 15},
		{key: "N", wantCur: 1, wantTop: 1 + 7},
		{key: "N", wantCur: 0, wantTop: 1},
		{key: "N", wantCur: 14, wantTop: 90},
		{key: "n", wantCur: 0, wantTop: 1},
	}
	for _, s := range steps {
		m, _ = keys(t, m, s.key)
		if m.search.cur != s.wantCur || m.top != s.wantTop {
			t.Fatalf("after %s: match %d at top %d, want %d at %d",
				s.key, m.search.cur, m.top, s.wantCur, s.wantTop)
		}
	}
}

func TestSearchScrollsSideways(t *testing.T) {
	text := strings.Repeat("x", 100) + "needle\n"
	m := open(t, "a.txt", text, WithSize(20, 3), WithLineNumbers(false))
	m, _ = keys(t, m, "/")
	m = typeText(t, m, "needle")
	m, _ = keys(t, m, "enter")
	if m.left != 95 {
		t.Errorf("left = %d, want 95", m.left)
	}
	if !strings.Contains(plain(m), "needle") {
		t.Errorf("the match isn't in view:\n%s", plain(m))
	}
}

func TestSearchInputCapturesKeys(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
	m, _ = keys(t, m, "/")
	if !m.Capturing() {
		t.Fatal("/ didn't open the search input")
	}
	m, msg := keys(t, m, "j", "q", "G")
	if m.top != 0 || msg != nil {
		t.Errorf("keys leaked out of the input: top %d, msg %v", m.top, msg)
	}
	if got := m.prompt.Value(); got != "jqG" {
		t.Errorf("input = %q, want %q", got, "jqG")
	}
	if got := m.ShortHelp(); len(got) != 2 {
		t.Errorf("help while searching has %d keys, want enter and esc", len(got))
	}
	m, msg = keys(t, m, "esc")
	if m.Capturing() || msg != nil || m.Query() != "" {
		t.Errorf("esc: capturing %v, msg %v, query %q; want the input closed and nothing else",
			m.Capturing(), msg, m.Query())
	}
}

func TestClose(t *testing.T) {
	m := open(t, "a.txt", "apple\n", WithSize(20, 3))
	for _, k := range []string{"q", "esc"} {
		_, msg := keys(t, m, k)
		if got, ok := msg.(CloseMsg); !ok || got.ID != m.ID() {
			t.Errorf("%s sent %#v, want CloseMsg{%d}", k, msg, m.ID())
		}
	}

	// With a search shown, esc clears it first.
	m, _ = keys(t, m, "/", "a", "enter")
	m, msg := keys(t, m, "esc")
	if msg != nil || m.Query() != "" || m.KeyMap().Next.Enabled() {
		t.Errorf("esc: msg %v, query %q; want the search cleared", msg, m.Query())
	}
	if _, msg = keys(t, m, "esc"); msg == nil {
		t.Error("a second esc didn't close")
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
	m.Blur()
	m, msg := keys(t, m, "j", "/", "q")
	if m.top != 0 || m.Capturing() || msg != nil {
		t.Errorf("a blurred pager reacted: top %d, capturing %v, msg %v", m.top, m.Capturing(), msg)
	}
}

func TestBlurClosesSearch(t *testing.T) {
	m := open(t, "a.txt", "a\n", WithSize(20, 3))
	m, _ = keys(t, m, "/")
	m.Blur()
	if m.Capturing() {
		t.Error("blur left the search input open")
	}
}

func TestHighlight(t *testing.T) {
	m := New(WithSize(40, 5))
	cmd := m.SetContent("main.go", goSource)
	if cmd == nil {
		t.Fatal("no highlight for a Go file")
	}
	msg := cmd()

	other := New()
	if other, _ = other.Update(msg); other.spans != nil {
		t.Error("another pager took the tokens")
	}
	stale := m
	stale.SetContent("main.go", goSource)
	if stale, _ = stale.Update(msg); stale.spans != nil {
		t.Error("tokens of older content were kept")
	}
	// Blurred pagers take their tokens too.
	if m, _ = m.Update(msg); len(m.spans) != m.Lines() {
		t.Errorf("%d lines of tokens for %d lines", len(m.spans), m.Lines())
	}
}

func TestHighlightSkipped(t *testing.T) {
	tests := []struct {
		name string
		file string
		opts []Option
	}{
		{name: "unknown syntax", file: "notes"},
		{name: "plain text", file: "notes.txt"},
		{name: "over the limit", file: "main.go", opts: []Option{WithHighlightLimit(10)}},
		{name: "turned off", file: "main.go", opts: []Option{WithHighlightLimit(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.opts...)
			if cmd := m.SetContent(tt.file, "just some words\n"); cmd != nil && cmd() != nil {
				t.Error("highlighted anyway")
			}
		})
	}
}

func TestHighlightCancelled(t *testing.T) {
	m := New()
	cmd := m.SetContent("main.go", strings.Repeat(goSource, 2000))
	_ = m.SetLoading("next.go")
	if msg := cmd(); msg != nil {
		t.Errorf("a cancelled highlight sent %T", msg)
	}
}

func TestStates(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Model)
		want state
	}{
		{name: "new", set: func(*Model) {}, want: stateEmpty},
		{name: "loading", set: func(m *Model) { _ = m.SetLoading("a.go") }, want: stateLoading},
		{name: "failed", set: func(m *Model) { m.SetError("a.go", errors.New("boom")) }, want: stateFailed},
		{name: "binary", set: func(m *Model) { m.SetContent("a.png", "\x89PNG\x00\x01") }, want: stateBinary},
		{name: "message", set: func(m *Model) { m.SetMessage("a.bin", "Too large.") }, want: stateMessage},
		{name: "ready", set: func(m *Model) { m.SetContent("a.go", "") }, want: stateReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "old.txt", numbered(50), WithSize(20, 5))
			m, _ = keys(t, m, "f", "/", "l", "enter")
			tt.set(&m)
			if tt.want == stateEmpty {
				return
			}
			if m.state != tt.want || m.top != 0 || m.Matches() != 0 || m.spans != nil {
				t.Errorf("state %d at top %d with %d matches; want %d, reset", m.state, m.top, m.Matches(), tt.want)
			}
		})
	}
}

func TestAdvance(t *testing.T) {
	tests := []struct {
		s        string
		i, cols  int
		wantJ    int
		wantUsed int
	}{
		{s: "abcdef", cols: 3, wantJ: 3, wantUsed: 3},
		{s: "abc", cols: 10, wantJ: 3, wantUsed: 3},
		{s: "abcdef", i: 2, cols: 2, wantJ: 4, wantUsed: 2},
		{s: "a你好", cols: 4, wantJ: 4, wantUsed: 3},
		{s: "a你好", cols: 5, wantJ: 7, wantUsed: 5},
		{s: "éx", cols: 1, wantJ: 3, wantUsed: 1},
		{s: "abc", cols: 0, wantJ: 0, wantUsed: 0},
	}
	for _, tt := range tests {
		j, used := advance(tt.s, tt.i, tt.cols)
		if j != tt.wantJ || used != tt.wantUsed {
			t.Errorf("advance(%q, %d, %d) = %d, %d; want %d, %d",
				tt.s, tt.i, tt.cols, j, used, tt.wantJ, tt.wantUsed)
		}
	}
}

func TestKeyMap(t *testing.T) {
	k := DefaultKeyMap()
	k.Close = key.NewBinding(key.WithKeys("x"))
	m := open(t, "a.txt", "a\n", WithKeyMap(k), WithSize(20, 3))
	if _, msg := keys(t, m, "q"); msg != nil {
		t.Error("q closed with a key map that binds x")
	}
	if _, msg := keys(t, m, "x"); msg == nil {
		t.Error("x didn't close")
	}
	var _ tea.Msg = CloseMsg{}
}

func TestHighlightGuessesSyntax(t *testing.T) {
	m := New()
	if cmd := m.SetContent("run", "#!/bin/sh\necho hi\n"); cmd == nil {
		t.Error("a script with a shebang wasn't highlighted")
	}
}

func TestHighlightSyntax(t *testing.T) {
	const patch = "@@ -1,2 +1,2 @@\n-old line\n+new line\n context\n"
	m := New()
	cmd := m.SetContentSyntax("main.go", "diff", patch)
	if cmd == nil {
		t.Fatal("a patch named after a Go file wasn't highlighted as a diff")
	}
	if m.Name() != "main.go" {
		t.Errorf("Name() = %q, want the name given", m.Name())
	}
	m, _ = m.Update(cmd())
	if len(m.spans) != m.Lines() {
		t.Fatalf("%d lines of tokens for %d lines", len(m.spans), m.Lines())
	}
	if got := m.spans[1][0].typ; got != chroma.GenericDeleted {
		t.Errorf("the removed line is %v, want %v", got, chroma.GenericDeleted)
	}
	if cmd := m.SetContentSyntax("main.go", "no-such-syntax", patch); cmd != nil && cmd() != nil {
		t.Error("an unknown syntax was highlighted")
	}
}

func TestSpinner(t *testing.T) {
	m := New(WithSize(20, 3))
	tick := m.SetLoading("a.go")()
	m, cmd := m.Update(tick)
	if cmd == nil {
		t.Fatal("the spinner stopped while loading")
	}
	m.SetContent("a.txt", "a\n")
	if _, cmd = m.Update(cmd()); cmd != nil {
		t.Error("the spinner kept going after the content arrived")
	}
}

func TestGoToLine(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
	m.GoToLine(50)
	// Ten rows of text, with line 50 a third of the way down.
	if m.top != 46 || m.mark != 49 {
		t.Errorf("top %d with mark %d, want 46 and line 50 marked", m.top, m.mark)
	}
	m.GoToLine(1000)
	if m.mark != 99 || m.top != 90 {
		t.Errorf("past the end: top %d with mark %d, want the last line", m.top, m.mark)
	}
	m.GoToLine(0)
	if m.mark != 99 {
		t.Error("line 0 moved the mark")
	}
	m.SetContent("other.txt", "text")
	if m.mark != -1 {
		t.Error("new content kept the mark")
	}
}

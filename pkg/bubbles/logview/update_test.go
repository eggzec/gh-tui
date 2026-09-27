package logview

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// numbered returns n plain lines that say their number.
func numbered(n int) []Line {
	out := make([]Line, n)
	for i := range out {
		out[i] = Line{Text: "line " + strconv.Itoa(i+1)}
	}
	return out
}

func TestScroll(t *testing.T) {
	// 100 lines in a window of 10 rows and the status line.
	tests := []struct {
		name             string
		keys             []string
		wantCur, wantTop int
	}{
		{name: "down", keys: []string{"j"}, wantCur: 1, wantTop: 0},
		{name: "down past the window", keys: slices.Repeat([]string{"down"}, 12), wantCur: 12, wantTop: 3},
		{name: "up at the top", keys: []string{"k", "up"}, wantCur: 0, wantTop: 0},
		{name: "page down", keys: []string{"f"}, wantCur: 10, wantTop: 10},
		{name: "pgdown", keys: []string{"pgdown", "pgdown"}, wantCur: 20, wantTop: 20},
		{name: "page up", keys: []string{"f", "f", "b"}, wantCur: 10, wantTop: 10},
		{name: "pgup", keys: []string{"j", "f", "pgup"}, wantCur: 1, wantTop: 0},
		{name: "half page down", keys: []string{"d"}, wantCur: 5, wantTop: 5},
		{name: "ctrl+d", keys: []string{"ctrl+d"}, wantCur: 5, wantTop: 5},
		{name: "half page up", keys: []string{"d", "d", "u"}, wantCur: 5, wantTop: 5},
		{name: "end", keys: []string{"G"}, wantCur: 99, wantTop: 90},
		{name: "end key", keys: []string{"end"}, wantCur: 99, wantTop: 90},
		{name: "home", keys: []string{"G", "g"}, wantCur: 0, wantTop: 0},
		{name: "home key", keys: []string{"f", "home"}, wantCur: 0, wantTop: 0},
		{name: "page down stops at the end", keys: slices.Repeat([]string{"f"}, 20), wantCur: 99, wantTop: 90},
		{name: "up from the end", keys: []string{"G", "k", "k"}, wantCur: 97, wantTop: 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := view(t, numbered(100), WithSize(40, 11))
			m, _ = keys(t, m, tt.keys...)
			if m.cur != tt.wantCur || m.top != tt.wantTop {
				t.Errorf("cursor %d, top %d; want %d, %d", m.cur, m.top, tt.wantCur, tt.wantTop)
			}
		})
	}
}

func TestScrollWrapped(t *testing.T) {
	// Every line takes three rows of 10 columns, after a gutter of 3.
	lines := slices.Repeat(plainLines(strings.Repeat("abcdefghij", 3)), 10)
	m := view(t, lines, WithSize(13, 5), WithLineNumbers(false), WithWrap(true))
	tests := []struct {
		keys                      []string
		wantCur, wantTop, wantRow int
	}{
		{keys: []string{"j"}, wantCur: 1, wantTop: 0, wantRow: 2},
		{keys: []string{"j", "j"}, wantCur: 2, wantTop: 1, wantRow: 2},
		{keys: []string{"j", "j", "k"}, wantCur: 1, wantTop: 1, wantRow: 0},
		// The window moves four rows and the cursor as many lines, as far as
		// the window shows, and then shows all of the cursor's line.
		{keys: []string{"f"}, wantCur: 2, wantTop: 1, wantRow: 2},
		// 30 rows and a window of 4 end at the second row of line 9.
		{keys: []string{"G"}, wantCur: 9, wantTop: 8, wantRow: 2},
		{keys: []string{"G", "g"}, wantCur: 0, wantTop: 0, wantRow: 0},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, " "), func(t *testing.T) {
			got, _ := keys(t, m, tt.keys...)
			if got.cur != tt.wantCur || got.top != tt.wantTop || got.row != tt.wantRow {
				t.Errorf("cursor %d at %d:%d, want %d at %d:%d",
					got.cur, got.top, got.row, tt.wantCur, tt.wantTop, tt.wantRow)
			}
			assertFits(t, got.View(), 13, 5)
		})
	}
}

func TestScrollSideways(t *testing.T) {
	// The gutter leaves 37 columns of text, a quarter of which is one
	// step.
	m := view(t, plainLines(strings.Repeat("0123456789", 5), "short"), WithSize(40, 5), WithLineNumbers(false))
	m, _ = keys(t, m, "l")
	if m.left != 9 {
		t.Errorf("left = %d, want 9", m.left)
	}
	m, _ = keys(t, m, "l", "l", "l")
	if m.left != 13 {
		t.Errorf("left = %d, want 13, where the longest line ends", m.left)
	}
	m, _ = keys(t, m, "h", "h", "h")
	if m.left != 0 {
		t.Errorf("left = %d, want 0", m.left)
	}
	m, _ = keys(t, m, "s", "l")
	if m.left != 0 || !m.Wrap() {
		t.Errorf("left = %d while wrapping, want 0", m.left)
	}
}

func TestErrors(t *testing.T) {
	m := open(t, WithSize(80, 24))
	m.CollapseAll()
	errText := func(i int) string { return m.rows[m.errs[i]].text }
	tests := []struct {
		name   string
		keys   []string
		want   int
		status string
	}{
		{name: "next expands what hides it", keys: []string{"e"}, want: 0, status: "error 1/5"},
		{name: "next", keys: []string{"e", "e", "e"}, want: 2, status: "error 3/5"},
		{name: "next wraps", keys: slices.Repeat([]string{"e"}, 6), want: 0, status: "error 1/5"},
		{name: "prev wraps", keys: []string{"E"}, want: 4, status: "error 5/5"},
		{name: "prev", keys: []string{"G", "E", "E"}, want: 3, status: "error 4/5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := keys(t, m, tt.keys...)
			if cursorText(got) != errText(tt.want) {
				t.Errorf("cursor on %q, want error %d", cursorText(got), tt.want+1)
			}
			if !got.covers(got.cur) {
				t.Error("the error is out of view")
			}
			if !strings.Contains(plain(got), tt.status) {
				t.Errorf("status doesn't say %q:\n%s", tt.status, plain(got))
			}
		})
	}
	if !strings.Contains(plain(m), "5 errors") {
		t.Errorf("status doesn't count the errors:\n%s", plain(m))
	}
}

func TestWarnings(t *testing.T) {
	m := open(t, WithSize(80, 24))
	m.CollapseAll()
	m, _ = keys(t, m, "w")
	if want := "Restore cache failed: "; !strings.HasPrefix(cursorText(m), want) {
		t.Errorf("cursor on %q, want the warning", cursorText(m))
	}
	if !strings.Contains(plain(m), "warning 1/1") {
		t.Errorf("status doesn't count the warning:\n%s", plain(m))
	}
	m, _ = keys(t, m, "W")
	if !strings.HasPrefix(cursorText(m), "Restore cache failed: ") {
		t.Errorf("W moved to %q, want the only warning", cursorText(m))
	}
}

// Keys without anything to go to are disabled, so help doesn't list them.
func TestKeysEnabled(t *testing.T) {
	m := view(t, plainLines("fine"), WithSize(40, 5))
	k := m.KeyMap()
	for _, b := range []key.Binding{k.NextError, k.PrevError, k.NextWarning, k.PrevWarning, k.Next, k.Prev} {
		if b.Enabled() {
			t.Errorf("%q is enabled without anything to go to", b.Help().Desc)
		}
	}
	m.Append(Line{Kind: Error, Text: "late"}, Line{Kind: Warning, Text: "later"})
	k = m.KeyMap()
	for _, b := range []key.Binding{k.NextError, k.PrevError, k.NextWarning, k.PrevWarning} {
		if !b.Enabled() {
			t.Errorf("%q is disabled after errors and warnings arrived", b.Help().Desc)
		}
	}
	if m.Errors() != 1 || m.Warnings() != 1 {
		t.Errorf("counts %d errors and %d warnings, want 1 and 1", m.Errors(), m.Warnings())
	}
}

func TestTimes(t *testing.T) {
	start := time.Date(2026, 9, 22, 9, 50, 10, 0, time.UTC)
	lines := []Line{
		{Time: start, Text: "one"},
		{Time: start.Add(1500 * time.Millisecond), Text: "two"},
		{Text: "no time"},
		{Time: start.Add(time.Hour + 2*time.Minute + 3*time.Second), Text: "three"},
		{Time: start.Add(2 * time.Hour), Text: "four"},
	}
	m := New(WithSize(60, 7), WithLineNumbers(false))
	m.Focus()
	m.SetLines(lines, []Section{{Title: "a", Start: 0, End: 1}, {Title: "b", Start: 1, End: 5}})
	want := map[TimeMode][]string{
		TimeHidden: {"a", "one", "b", "two", "no time", "three"},
		// b starts with two.
		TimeRelative: {"a", "+00:00.0   one", "b", "+00:00.0   two", "no time", "+1:02:01   three"},
		TimeAbsolute: {"a", "09:50:10   one", "b", "09:50:11   two", "no time", "10:52:13   three"},
	}
	for _, mode := range []TimeMode{TimeRelative, TimeAbsolute, TimeHidden} {
		m, _ = keys(t, m, "t")
		if m.TimeMode() != mode {
			t.Fatalf("t moved to %v, want %v", m.TimeMode(), mode)
		}
		rows := strings.Split(plain(m), "\n")
		for i, w := range want[mode] {
			if got := strings.Join(strings.Fields(rows[i]), " "); !strings.Contains(got, strings.Join(strings.Fields(w), " ")) {
				t.Errorf("mode %v row %d = %q, want %q", mode, i, got, w)
			}
		}
	}
}

func TestAppendSince(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                            "+00:00.0",
		1500 * time.Millisecond:                 "+00:01.5",
		61 * time.Second:                        "+01:01.0",
		time.Hour + 2*time.Minute + time.Second: "+1:02:01",
		12*time.Hour + 5*time.Second:            "+12:00:05",
	} {
		if got := string(appendSince(nil, d)); got != want {
			t.Errorf("appendSince(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestAppendDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		400 * time.Millisecond:    "0s",
		42 * time.Second:          "42s",
		65 * time.Second:          "1m 5s",
		time.Hour + time.Minute*2: "1h 2m",
	} {
		if got := string(appendDuration(nil, d)); got != want {
			t.Errorf("appendDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestSearch(t *testing.T) {
	m := open(t, WithSize(80, 24))
	m.CollapseAll()
	m = find(t, m, "unused")
	// Four errors say so, and each has it twice; the summary once more.
	if m.Matches() != 9 {
		t.Fatalf("found %d matches, want 9", m.Matches())
	}
	if !strings.Contains(cursorText(m), "clipboardCommand is unused") {
		t.Errorf("cursor on %q, want the first match", cursorText(m))
	}
	if !isOpen(t, m, "golangci-lint") || !isOpen(t, m, "run golangci-lint") {
		t.Error("the folds of the match are collapsed")
	}
	if !strings.Contains(plain(m), "match 1/9") {
		t.Errorf("status doesn't count the match:\n%s", plain(m))
	}
	m, _ = keys(t, m, "n", "n")
	if m.search.cur != 2 || !strings.Contains(cursorText(m), "commandClipboardBackend is unused") {
		t.Errorf("n n went to match %d on %q", m.search.cur+1, cursorText(m))
	}
	m, _ = keys(t, m, "N", "N", "N")
	if m.search.cur != 8 || cursorText(m) != "* unused: 4" {
		t.Errorf("N wrapped to match %d on %q, want the last", m.search.cur+1, cursorText(m))
	}
	// esc clears the search, and then asks to close.
	m, msg := keys(t, m, "esc")
	if m.Query() != "" || msg != nil {
		t.Errorf("esc left query %q and sent %v", m.Query(), msg)
	}
	_, msg = keys(t, m, "esc")
	if msg != (CloseMsg{ID: m.ID()}) {
		t.Errorf("second esc sent %v, want a CloseMsg", msg)
	}
}

func TestSearchCase(t *testing.T) {
	m := view(t, plainLines("Error here", "error there", "no"), WithSize(40, 5))
	if m = find(t, m, "error"); m.Matches() != 2 {
		t.Errorf("lowercase query found %d, want 2 ignoring case", m.Matches())
	}
	if m = find(t, m, "Error"); m.Matches() != 1 {
		t.Errorf("query with a capital found %d, want 1", m.Matches())
	}
	if m = find(t, m, "kiwi"); m.Matches() != 0 || !strings.Contains(plain(m), "no matches") {
		t.Errorf("query without matches: %d, %q", m.Matches(), plain(m))
	}
	// Matches ignore the log's colors.
	m = view(t, []Line{{Text: "\x1b[31mfa\x1b[0mil"}}, WithSize(40, 5))
	if m = find(t, m, "fail"); m.Matches() != 1 {
		t.Errorf("colored text found %d, want 1", m.Matches())
	}
}

func TestSearchInput(t *testing.T) {
	m := view(t, plainLines("abc"), WithSize(40, 5))
	m, _ = keys(t, m, "/")
	if !m.Capturing() {
		t.Fatal("the search input isn't capturing")
	}
	m = typeText(t, m, "q")
	if m.Capturing() != true {
		t.Error("q closed the input, want it typed")
	}
	m, msg := keys(t, m, "esc")
	if m.Capturing() || msg != nil || m.Query() != "" {
		t.Errorf("esc in the input: capturing %v, sent %v, query %q", m.Capturing(), msg, m.Query())
	}
	m.Blur()
	if m, msg = keys(t, m, "j", "/", "q"); m.Capturing() || msg != nil {
		t.Error("a blurred view reacted to keys")
	}
}

func TestAppend(t *testing.T) {
	m := New(WithSize(40, 6))
	m.Focus()
	m.SetLines(numbered(3), []Section{{Title: "build", Start: 0, End: 3}})
	m.Append(Line{Kind: Group, Text: "details"}, Line{Text: "hidden"})
	m.Append(Line{Text: "still hidden"}, Line{Kind: EndGroup}, Line{Text: "shown"})
	want := []string{"build", ".line 1", ".line 2", ".line 3", ".details", ".shown"}
	if got := shownRows(m); !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
	m, _ = keys(t, m, "*")
	if got := len(m.vis); got != 8 {
		t.Errorf("expand all shows %d rows, want 8", got)
	}
	if m.Lines() != 8 {
		t.Errorf("Lines() = %d, want 8", m.Lines())
	}
}

func TestAppendFollows(t *testing.T) {
	tests := []struct {
		name       string
		keys       []string
		wantCursor string
	}{
		{name: "at the end it follows", keys: []string{"G"}, wantCursor: "line 23"},
		{name: "above the end it stays", keys: []string{"G", "k"}, wantCursor: "line 19"},
		{name: "F turns follow off", keys: []string{"G", "F"}, wantCursor: "line 20"},
		{name: "F back on goes to the end", keys: []string{"F", "g", "F"}, wantCursor: "line 23"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := view(t, numbered(20), WithSize(40, 6))
			m, _ = keys(t, m, tt.keys...)
			m.Append(numbered(23)[20:]...)
			if got := cursorText(m); got != tt.wantCursor {
				t.Errorf("cursor on %q, want %q", got, tt.wantCursor)
			}
			if !m.covers(m.cur) {
				t.Error("the cursor is out of view")
			}
		})
	}
}

func TestAppendSearches(t *testing.T) {
	m := view(t, plainLines("fail one"), WithSize(40, 6))
	m = find(t, m, "fail")
	m.Append(Line{Text: "fail two"}, Line{Kind: Error, Text: "failed"})
	if m.Matches() != 3 {
		t.Errorf("found %d matches after appending, want 3", m.Matches())
	}
	if v := plain(m); !strings.Contains(v, "follow") || !strings.Contains(v, "match 1/3") {
		t.Errorf("status of a live log:\n%s", v)
	}
}

// Appending to a view that was never given lines starts a log.
func TestAppendFirst(t *testing.T) {
	m := New(WithSize(40, 4))
	_ = m.SetLoading()
	m.Append(plainLines("first")...)
	if got := shownRows(m); !slices.Equal(got, []string{"first"}) {
		t.Errorf("rows = %q, want the appended line", got)
	}
	m.Append()
	if m.Lines() != 1 {
		t.Errorf("Lines() = %d after appending nothing, want 1", m.Lines())
	}
}

func TestToggles(t *testing.T) {
	m := view(t, plainLines("a"), WithSize(40, 4))
	m, _ = keys(t, m, "s", "#")
	if !m.Wrap() || m.LineNumbers() {
		t.Errorf("wrap %v, line numbers %v; want true, false", m.Wrap(), m.LineNumbers())
	}
	m, _ = keys(t, m, "s", "#", "F")
	if m.Wrap() || !m.LineNumbers() || m.Follow() {
		t.Errorf("wrap %v, line numbers %v, follow %v; want false, true, false", m.Wrap(), m.LineNumbers(), m.Follow())
	}
}

func TestLoading(t *testing.T) {
	m := New(WithSize(40, 4))
	tick := m.SetLoading()
	if tick == nil {
		t.Fatal("SetLoading returned no command")
	}
	m, cmd := m.Update(tick())
	if cmd == nil {
		t.Error("the spinner stopped while loading")
	}
	m.SetLines(plainLines("done"), nil)
	if _, cmd = m.Update(tick()); cmd != nil {
		t.Error("the spinner kept spinning after the lines arrived")
	}
}

func TestHelp(t *testing.T) {
	m := open(t, WithSize(80, 24))
	if len(m.ShortHelp()) == 0 || len(m.FullHelp()) != 4 {
		t.Errorf("help has %d short keys and %d columns", len(m.ShortHelp()), len(m.FullHelp()))
	}
	m, _ = keys(t, m, "/")
	if got := m.ShortHelp(); len(got) != 2 {
		t.Errorf("help while searching lists %d keys, want confirm and cancel", len(got))
	}
	var enabled []string
	for _, g := range m.FullHelp() {
		for _, b := range g {
			if b.Enabled() {
				enabled = append(enabled, b.Help().Key)
			}
		}
	}
	if !slices.Equal(enabled, []string{"enter", "esc"}) {
		t.Errorf("full help while searching enables %q, want enter and esc", enabled)
	}
}

// Messages that aren't keys go to the input while it is open, and are
// ignored otherwise.
func TestUpdatePaste(t *testing.T) {
	m := view(t, plainLines("abc"), WithSize(40, 5))
	m, _ = keys(t, m, "/")
	m, _ = m.Update(tea.PasteMsg{Content: "ab"})
	if got := m.input.Value(); got != "ab" {
		t.Errorf("input = %q, want the paste", got)
	}
	m, _ = keys(t, m, "enter")
	if m.Matches() != 1 {
		t.Errorf("found %d, want 1", m.Matches())
	}
	if _, cmd := m.Update(tea.PasteMsg{Content: "x"}); cmd != nil {
		t.Error("a paste outside the input did something")
	}
}

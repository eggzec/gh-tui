package cmdline

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// longLine is wider than a 120-column terminal.
const longLine = "search is:open is:pr author:@me review-requested:@me " +
	"label:bug label:\"help wanted\" repo:cli/cli repo:charmbracelet/bubbletea sort:updated-desc"

func TestView(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		initial string
		keys    []tea.Msg
		blurred bool
		// height is the most rows it may take, MaxHeight if 0.
		height int
	}{
		{name: "empty"},
		{name: "placeholder", opts: []Option{WithPlaceholder("goto owner/repo")}},
		{name: "typed", initial: "goto cli/cli"},
		{name: "cursor inside", initial: "goto cli/cli", keys: []tea.Msg{left, left, left}},
		{name: "long line scrolls", initial: longLine},
		{name: "long line from the start", initial: longLine, keys: []tea.Msg{home}},
		{name: "prompt", initial: "cli/cli", opts: []Option{WithPrompt("goto ")}},
		{name: "blurred", initial: "goto cli/cli", blurred: true},
		{name: "light", initial: "goto cli/cli", opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "candidates", initial: "goto gammons/sl", opts: []Option{WithComplete(repoComplete)}},
		{name: "candidate selected", initial: "goto gammons/sl", keys: []tea.Msg{tab, tab},
			opts: []Option{WithComplete(repoComplete)}},
		{name: "candidates light", initial: "goto gammons/sl", keys: []tea.Msg{tab},
			opts: []Option{WithComplete(repoComplete), WithStyles(DefaultStyles(false))}},
		{name: "candidates with details", initial: "", opts: []Option{WithComplete(commandComplete)}},
		{name: "detail selected", initial: "", keys: []tea.Msg{tab, tab},
			opts: []Option{WithComplete(commandComplete)}},
		{name: "candidates overflow", initial: "goto repo", opts: []Option{WithComplete(manyComplete)}},
		{name: "candidates scrolled", initial: "goto repo", keys: []tea.Msg{shiftTab, shiftTab},
			opts: []Option{WithComplete(manyComplete)}},
		{name: "candidates scrolled to the end", initial: "goto repo", keys: []tea.Msg{shiftTab},
			opts: []Option{WithComplete(manyComplete)}},
		{name: "wide candidate is cut", initial: "open ", opts: []Option{WithComplete(wideComplete)}},
		{name: "one row drops candidates", initial: "goto gammons/sl", height: 1,
			opts: []Option{WithComplete(repoComplete)}},
		{name: "long line with candidates", initial: longLine + " repo:gam",
			opts: []Option{WithComplete(repoComplete)}},
	}
	for _, width := range []int{80, 120} {
		for _, tt := range tests {
			t.Run(strconv.Itoa(width)+"/"+tt.name, func(t *testing.T) {
				height := MaxHeight
				if tt.height > 0 {
					height = tt.height
				}
				m := opened(t, tt.initial, append(tt.opts, WithSize(width, height))...)
				m, _ = press(t, m, tt.keys...)
				if tt.blurred {
					m.Blur()
				}
				v := m.View()
				assertFits(t, v, width, m.Height())
				golden.RequireEqual(t, v)
			})
		}
	}
}

var repoComplete = completeWords(repos...)

// commandComplete offers the commands, with what they do, while the line
// is empty.
func commandComplete(line string, _ int) []Candidate {
	if line != "" {
		return nil
	}
	cmds := [][2]string{
		{"goto", "open a repository"}, {"search", "search GitHub"},
		{"theme", "change the colors"}, {"quit", "leave gh-tui"},
	}
	out := make([]Candidate, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, Candidate{Text: c[0] + " ", Label: c[0], Detail: c[1]})
	}
	return out
}

// manyComplete offers more repos than a row can show.
var manyComplete = func() Complete {
	many := make([]string, 0, 26)
	for _, r := range "abcdefghijklmnopqrstuvwxyz" {
		many = append(many, "repo-"+string(r))
	}
	return completeWords(many...)
}()

// wideComplete offers one path wider than any row.
func wideComplete(_ string, cursor int) []Candidate {
	return []Candidate{{
		Text:  strings.Repeat("very/deep/", 16) + "file.go",
		Label: strings.Repeat("very/deep/", 16) + "file.go",
		Start: cursor, End: cursor,
	}}
}

// Every size renders exactly its width and height.
func TestViewFits(t *testing.T) {
	for _, w := range []int{0, 1, 2, 3, 10, 80, 200} {
		for _, h := range []int{0, 1, 2, 3} {
			t.Run(strconv.Itoa(w)+"x"+strconv.Itoa(h), func(t *testing.T) {
				m := opened(t, longLine+" repo", WithSize(w, h), WithComplete(manyComplete))
				assertFits(t, m.View(), w, m.Height())
				for range 5 {
					m, _ = m.Update(shiftTab)
					assertFits(t, m.View(), w, m.Height())
				}
				m.SetSize(h*7, w%3)
				assertFits(t, m.View(), h*7, m.Height())
			})
		}
	}
}

// The row is rendered again only when the candidates, the width or the
// selection change.
func TestViewCachesRow(t *testing.T) {
	m := opened(t, "goto gammons/", WithSize(80, MaxHeight), WithComplete(repoComplete))
	items := &m.comp.items[0]
	m = typeText(t, m, "s")
	if &m.comp.items[0] != items {
		t.Error("the same candidates were rendered again")
	}
	row := m.comp.row
	m, _ = m.Update(tab)
	if m.comp.row == row {
		t.Error("the row didn't show the selection")
	}
	m = typeText(t, m, "-")
	if &m.comp.items[0] == items {
		t.Error("new candidates weren't rendered")
	}
}

// The cursor stays in view wherever it goes on a long line.
func TestViewCursorInView(t *testing.T) {
	m := opened(t, longLine, WithSize(40, MaxHeight))
	for i := range len(longLine) {
		assertCursorShown(t, m, 40)
		if t.Failed() {
			t.Fatalf("after %d moves left", i)
		}
		m, _ = m.Update(left)
	}
}

// The line scrolls again when the command line shrinks or grows, so the
// cursor stays in view.
func TestViewCursorInViewOnResize(t *testing.T) {
	for _, moves := range []int{0, 10, 60, len(longLine)} {
		t.Run(strconv.Itoa(moves), func(t *testing.T) {
			m := opened(t, longLine, WithSize(120, MaxHeight))
			for range moves {
				m, _ = m.Update(left)
			}
			for _, w := range []int{40, 20, 80, 200, 30} {
				m.SetSize(w, MaxHeight)
				assertCursorShown(t, m, w)
				if t.Failed() {
					t.Fatalf("at width %d", w)
				}
			}
			// Grown wide enough, the line shows whole again.
			m.SetSize(200, MaxHeight)
			if !strings.Contains(ansi.Strip(m.View()), ":"+longLine) {
				t.Errorf("at 200 columns the line isn't whole:\n%s", ansi.Strip(m.View()))
			}
		})
	}
}

// A parent lays out again after every Update, at the same size. That
// leaves the scroll alone, so the cursor moves as it would without it.
func TestViewRelayoutKeepsScroll(t *testing.T) {
	keys := make([]tea.Msg, 0, 20)
	keys = append(keys, home)
	for range 10 {
		keys = append(keys, tea.KeyPressMsg{Code: tea.KeyRight})
	}
	for _, r := range "hello" {
		keys = append(keys, runeKey(string(r)))
	}
	keys = append(keys, bksp, bksp, left, left)
	// Two command lines, not copies of one: copies share the input's runes.
	plain := opened(t, longLine, WithSize(40, MaxHeight))
	relaid := opened(t, longLine, WithSize(40, MaxHeight))
	for i, k := range keys {
		plain, _ = plain.Update(k)
		relaid, _ = relaid.Update(k)
		relaid.SetSize(40, MaxHeight)
		if a, b := cursorColumn(t, plain), cursorColumn(t, relaid); a != b {
			t.Fatalf("after key %d the cursor is at column %d with a relayout, %d without", i, b, a)
		}
		if plain.View() != relaid.View() {
			t.Fatalf("after key %d a relayout changed the view:\n%s\n%s",
				i, ansi.Strip(plain.View()), ansi.Strip(relaid.View()))
		}
	}
	if c := cursorColumn(t, plain); c < 10 {
		t.Errorf("the cursor ends at column %d, want past the ten moves right", c)
	}
}

// The terminal's focus doesn't reach the input: the parent owns focus.
func TestViewIgnoresTerminalFocus(t *testing.T) {
	m := opened(t, "goto cli/cli", WithSize(40, MaxHeight))
	want := m.View()
	m, _ = press(t, m, tea.BlurMsg{})
	// A resize renders the line again.
	m.SetSize(41, MaxHeight)
	m.SetSize(40, MaxHeight)
	assertCursorShown(t, m, 40)
	m, _ = press(t, m, tea.FocusMsg{})
	if m.View() != want {
		t.Errorf("after the terminal's blur and focus the view is\n%q\nwant\n%q", m.View(), want)
	}
}

// cursorColumn returns the column of the cursor on the last line.
func cursorColumn(t *testing.T, m Model) int {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	line := lines[len(lines)-1]
	before, _, ok := strings.Cut(line, "\x1b[7")
	if !ok {
		t.Fatalf("no cursor in %q", ansi.Strip(line))
	}
	return ansi.StringWidth(before)
}

// assertCursorShown checks that the last line of the view shows the cursor
// on a cell inside width and isn't cut. A cut keeps the cursor's escape
// sequence after the text it styled, so finding the sequence isn't enough.
func assertCursorShown(t *testing.T, m Model, width int) {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	line := lines[len(lines)-1]
	if strings.Contains(ansi.Strip(line), "…") {
		t.Errorf("the line is cut: %q", ansi.Strip(line))
	}
	i := strings.Index(line, "\x1b[7")
	if i < 0 {
		t.Errorf("no cursor in %q", ansi.Strip(line))
		return
	}
	if at := ansi.StringWidth(line[:i]); at >= width {
		t.Errorf("the cursor is at column %d of %d", at, width)
	}
	rest := line[i+strings.IndexByte(line[i:], 'm')+1:]
	if rest == "" || rest[0] == '\x1b' {
		t.Errorf("the cursor styles no cell: %q", line[i:])
	}
}

// A placeholder of wide characters fills the line like any other.
func TestViewWidePlaceholder(t *testing.T) {
	m := opened(t, "", WithSize(40, MaxHeight), WithPlaceholder("リポジトリへ\n移動"))
	v := m.View()
	assertFits(t, v, 40, 1)
	if strings.ContainsRune(v, 0) {
		t.Errorf("the view has NUL bytes: %q", v)
	}
	if s := ansi.Strip(v); !strings.HasPrefix(s, ":リポジトリへ 移動") {
		t.Errorf("view is %q", s)
	}
}

// A value set before the size scrolls again once the line has room.
func TestViewScrollsAgainOnResize(t *testing.T) {
	m := New(testHistoryLimit, WithValue("goto cli/cli"))
	m.Open(m.Value())
	m.SetSize(40, MaxHeight)
	if v := ansi.Strip(m.View()); !strings.Contains(v, ":goto cli/cli") {
		t.Errorf("the value isn't in view after a resize:\n%s", v)
	}
}

func TestViewFollowsFocus(t *testing.T) {
	m := New(testHistoryLimit, WithValue("goto"), WithSize(20, MaxHeight))
	blurred := m.View()
	m.Focus()
	if m.View() == blurred {
		t.Error("focus didn't change the view")
	}
	m.Blur()
	if m.View() != blurred {
		t.Error("blur didn't restore the view")
	}
}

package picker

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func manyItems(n int) []Item {
	items := make([]Item, n)
	for i := range items {
		kind := kindRepos
		if i >= n/2 {
			kind = kindIssues
		}
		items[i] = Item{Kind: kind, Title: fmt.Sprintf("octo-org/project-%02d", i), Detail: "Tools for the terminal"}
	}
	return items
}

var markable = []Item{
	{Title: "alpha", Detail: "first", Value: "a"},
	{Title: "a title that is far too long to fit next to its mark in the row", Value: "b"},
	{Title: "gamma", Value: "c"},
}

func useTyped(text string) (Item, bool) {
	return Item{Title: `use "` + text + `"`}, true
}

// asciiStyles returns styles that draw ASCII alone.
func asciiStyles() Styles {
	st := DefaultStyles(true)
	st.Frame = st.Frame.Border(lipgloss.ASCIIBorder())
	st.PromptGlyph, st.CursorGlyph, st.Ellipsis = ">", ">", "..."
	return st
}

func TestView(t *testing.T) {
	scopes := WithScopes(kindRepos, kindIssues, kindPulls)
	tests := []struct {
		name          string
		search        bool
		fail          bool
		opts          []Option
		width, height int
		// skipInit leaves the first search running.
		skipInit bool
		typed    string
		keys     []any
		// marked is passed to SetMarked when set.
		marked []any
		// ascii checks the view holds nothing but ASCII.
		ascii bool
	}{
		{name: "searching", search: true, skipInit: true, width: 60, height: 10},
		{name: "first results", search: true, width: 60, height: 10},
		{name: "grouped with scopes", search: true, opts: []Option{scopes}, typed: "crash", width: 64, height: 10},
		{name: "scope in use", search: true, opts: []Option{scopes}, typed: "crash", keys: []any{tab, tab}, width: 64, height: 10},
		{name: "second selected", search: true, typed: "crash", keys: []any{down}, width: 60, height: 10},
		{name: "no results", search: true, typed: "zzz", width: 60, height: 8},
		{name: "error", search: true, fail: true, typed: "crash", width: 60, height: 8},
		{name: "fuzzy highlight", opts: []Option{WithItems(catalog)}, typed: "ghtui", width: 60, height: 8},
		{name: "scrolled", opts: []Option{WithItems(manyItems(20))}, keys: []any{pgDown, down, down}, width: 50, height: 10},
		{name: "no headers", opts: []Option{WithItems(catalog), WithGroupHeaders(false)}, width: 60, height: 8},
		{name: "narrow", search: true, opts: []Option{scopes}, typed: "crash", width: 30, height: 9},
		{name: "light", search: true, opts: []Option{WithStyles(DefaultStyles(false))}, typed: "crash", width: 60, height: 8},
		{name: "placeholder", opts: []Option{WithItems(nil), WithPlaceholder("Search repositories, issues and pull requests"), WithEmptyText("Type to search GitHub.")}, width: 60, height: 6},
		{name: "two rows", search: true, typed: "crash", width: 40, height: 4},
		{name: "one row", search: true, typed: "crash", width: 40, height: 3},
		{name: "normal mode", opts: []Option{WithItems(catalog), WithModes(true)}, keys: []any{down}, width: 60, height: 10},
		{name: "normal mode ascii", opts: []Option{WithItems(catalog), WithModes(true), WithStyles(asciiStyles())}, keys: []any{down}, width: 60, height: 10, ascii: true},
		{name: "marks", opts: []Option{WithItems(markable), WithMarks("[x]", "[ ]"), WithModes(true)}, keys: []any{down}, marked: []any{"a", "c"}, width: 50, height: 9},
		{name: "marks ascii", opts: []Option{WithItems(markable), WithMarks("[x]", "[ ]"), WithStyles(asciiStyles())}, marked: []any{"b"}, width: 40, height: 9, ascii: true},
		{name: "typed item", opts: []Option{WithItems(catalog), WithTyped(useTyped)}, typed: "crash", width: 60, height: 10},
		{name: "no filter line", opts: []Option{WithItems(catalog), WithModes(true), WithFilterLine(false)}, keys: []any{down}, width: 50, height: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var search Search
			if tt.search {
				f := &fakeSearch{}
				if tt.fail {
					f.fail = errBoom
				}
				search = f.search
			}
			m := New(search, append([]Option{WithKeyMap(testKeys(t))}, append(tt.opts, WithDebounce(0), WithSize(tt.width, tt.height))...)...)
			m.Focus()
			if !tt.skipInit {
				m, _ = run(t, m, m.Init())
			}
			if tt.marked != nil {
				m.SetMarked(tt.marked)
			}
			m = typeText(t, m, tt.typed)
			for _, k := range tt.keys {
				m, _ = press(t, m, k)
			}
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			if tt.ascii && strings.ContainsFunc(ansi.Strip(v), func(r rune) bool { return r > unicode.MaxASCII }) {
				t.Errorf("view isn't ASCII:\n%s", ansi.Strip(v))
			}
			golden.RequireEqual(t, v)
		})
	}
}

// Every size renders exactly its width and height.
func TestViewFits(t *testing.T) {
	f := &fakeSearch{}
	for _, w := range []int{1, 2, 3, 4, 5, 10, 40, 80, 200} {
		for _, h := range []int{1, 2, 3, 4, 5, 12, 40} {
			t.Run(strconv.Itoa(w)+"x"+strconv.Itoa(h), func(t *testing.T) {
				m := open(t, f.search, WithSize(w, h), WithScopes(kindRepos, kindIssues, kindPulls))
				m = typeText(t, m, "crash")
				assertFits(t, m.View(), w, h)
				m.SetSize(h*3, w%7+1)
				assertFits(t, m.View(), h*3, w%7+1)
			})
		}
	}
}

func TestViewFollowsFocus(t *testing.T) {
	m := New(nil, WithKeyMap(testKeys(t)), WithItems(catalog), WithSize(40, 6))
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

func TestViewCleansItems(t *testing.T) {
	m := New(nil, WithKeyMap(testKeys(t)), WithItems([]Item{{Title: "two\nlines \x1b[31mred\x1b[m", Detail: "tab\tdetail"}}), WithSize(40, 5))
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "two lines red  tab detail") {
		t.Errorf("the item isn't on one clean line:\n%s", v)
	}
}

func TestHighlight(t *testing.T) {
	base, match := DefaultStyles(true).Title, DefaultStyles(true).Match
	tests := []struct {
		s       string
		matches []int
		want    string
	}{
		{"gh-tui", nil, base.Render("gh-tui")},
		{"gh-tui", []int{0, 1}, match.Render("gh") + base.Render("-tui")},
		{"gh-tui", []int{3, 5}, base.Render("gh-") + match.Render("t") + base.Render("u") + match.Render("i")},
		{"añb", []int{1, 3}, base.Render("a") + match.Render("ñb")},
	}
	for _, tt := range tests {
		if got := highlight(tt.s, tt.matches, base, match); got != tt.want {
			t.Errorf("highlight(%q, %v) = %q, want %q", tt.s, tt.matches, got, tt.want)
		}
	}
}

func assertFits(t *testing.T, v string, width, height int) {
	t.Helper()
	if width == 0 || height == 0 {
		if v != "" {
			t.Fatalf("view = %q, want empty", v)
		}
		return
	}
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, v)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

// A picker with ASCII glyphs and frame is ASCII alone, cut rows and
// placeholder too.
func TestViewASCII(t *testing.T) {
	st := asciiStyles()
	items := []Item{{Title: strings.Repeat("a long title ", 10)}, {Title: "short"}}
	m := New(nil, WithKeyMap(testKeys(t)), WithItems(items), WithStyles(st), WithSize(30, 8))
	m.Focus()
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Search...") || !strings.Contains(v, "...") {
		t.Errorf("view lacks the placeholder or a cut row:\n%s", v)
	}
	if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
		t.Errorf("view isn't ASCII:\n%s", v)
	}
}

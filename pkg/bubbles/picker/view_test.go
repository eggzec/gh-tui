package picker

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

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
			m := New(search, append(tt.opts, WithDebounce(0), WithSize(tt.width, tt.height))...)
			m.Focus()
			if !tt.skipInit {
				m, _ = run(t, m, m.Init())
			}
			m = typeText(t, m, tt.typed)
			for _, k := range tt.keys {
				m, _ = press(t, m, k)
			}
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
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
	m := New(nil, WithItems(catalog), WithSize(40, 6))
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
	m := New(nil, WithItems([]Item{{Title: "two\nlines \x1b[31mred\x1b[m", Detail: "tab\tdetail"}}), WithSize(40, 5))
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

package feed

import (
	"errors"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	long := newSource(30, 10)
	for i := range long.items {
		long.items[i].title = strings.Repeat("a long pull request title that goes on ", 4)
	}
	failing := newSource(10, 10)
	failing.setFail("", errors.New("GET /repos/o/r/pulls: 502 Bad Gateway"))

	tests := []struct {
		name  string
		model func(t *testing.T) Model[item]
	}{
		{"loading", func(*testing.T) Model[item] {
			return New(newSource(10, 10).fetch, renderItem, WithSize(40, 5), WithFocused(true))
		}},
		{"loaded", func(t *testing.T) Model[item] {
			t.Helper()
			return load(t, newSource(30, 10))
		}},
		{"selection", func(t *testing.T) Model[item] {
			t.Helper()
			return keys(t, load(t, newSource(30, 10)), "down", "down", "pgdown")
		}},
		{"blurred", func(t *testing.T) Model[item] {
			t.Helper()
			m := keys(t, load(t, newSource(30, 10)), "down")
			m.Blur()
			return m
		}},
		{"loading more", func(t *testing.T) Model[item] {
			t.Helper()
			m := load(t, newSource(30, 10))
			m, _ = m.Update(press("end"))
			return m
		}},
		{"error", func(t *testing.T) Model[item] {
			t.Helper()
			return load(t, failing, WithSize(60, 5))
		}},
		{"error after items", func(t *testing.T) Model[item] {
			t.Helper()
			src := newSource(30, 10)
			src.setFail("10", errors.New("API rate limit exceeded"))
			return keys(t, load(t, src, WithSize(60, 5)), "end")
		}},
		{"empty", func(t *testing.T) Model[item] {
			t.Helper()
			return load(t, newSource(0, 10), WithEmptyText("No open pull requests. Press / to change the filter."))
		}},
		{"truncated at 80 columns", func(t *testing.T) Model[item] {
			t.Helper()
			return load(t, long, WithSize(80, 4))
		}},
		{"refetching", func(t *testing.T) Model[item] {
			t.Helper()
			m := scrolledToEnd(t, newSource(1000, 10))
			m, _ = m.Update(press("home"))
			return m
		}},
		{"refetch error", func(t *testing.T) Model[item] {
			t.Helper()
			src := newSource(1000, 10)
			m := scrolledToEnd(t, src)
			src.setFail("", errors.New("dial tcp: i/o timeout"))
			return keys(t, m, "home", "down")
		}},
		{"tall rows", func(t *testing.T) Model[item] {
			t.Helper()
			tall := func(it item, _ bool, _ int) string {
				return "#" + it.id + " " + it.title + "\n  opened by octocat\nthird line is dropped"
			}
			src := newSource(30, 10)
			m := New(src.fetch, tall, WithSize(40, 7), WithItemHeight(2), WithFocused(true))
			return keys(t, run(t, m, m.Init()), "down")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			golden.RequireEqual(t, v)
		})
	}
}

// assertFits checks that v is exactly height lines of exactly width cells.
func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Fatalf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Fatalf("line %d is %d cells wide, want %d: %q", i, w, width, l)
		}
	}
}

func TestViewFitsAnySize(t *testing.T) {
	m := load(t, newSource(30, 10))
	for _, size := range [][2]int{{1, 1}, {2, 3}, {3, 1}, {80, 40}, {200, 2}} {
		m.SetSize(size[0], size[1])
		assertFits(t, m.View(), size[0], size[1])
	}
	m.SetSize(0, 10)
	if m.View() != "" {
		t.Fatal("zero width should render nothing")
	}
}

// The cursor, the ellipsis of cut rows and placeholders, and the marks of
// errors are the glyphs of the styles, so a view with ASCII glyphs draws
// ASCII alone.
func TestViewGlyphs(t *testing.T) {
	st := DefaultStyles(true)
	st.CursorGlyph, st.Ellipsis = ">", "..."
	st.ErrorGlyph, st.ErrorSeparator, st.ErrorEllipsis = "x", " - ", "..."
	long := newSource(30, 10)
	for i := range long.items {
		long.items[i].title = strings.Repeat("a long title ", 4)
	}
	cut := ansi.Strip(load(t, long, WithSize(30, 3), WithStyles(st)).View())
	src := newSource(1000, 10)
	m := scrolledToEnd(t, src)
	m.SetStyles(st)
	m, _ = m.Update(press("home"))
	refetching := ansi.Strip(m.View())
	src = newSource(1000, 10)
	m = scrolledToEnd(t, src)
	m.SetStyles(st)
	src.setFail("", errors.New("dial tcp: i/o timeout"))
	failed := ansi.Strip(keys(t, m, "home", "down").View())
	for _, tt := range []struct{ view, want string }{
		{cut, "> "},
		{cut, "..."},
		{refetching, "  ..."},
		{failed, "x "},
	} {
		if !strings.Contains(tt.view, tt.want) {
			t.Errorf("view lacks %q:\n%s", tt.want, tt.view)
		}
	}
	for _, v := range []string{cut, refetching, failed} {
		if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("view has glyphs beyond ASCII:\n%s", v)
		}
	}
	loading := New(newSource(10, 10).fetch, renderItem, WithSize(40, 5), WithStyles(st))
	if v := ansi.Strip(loading.View()); !strings.Contains(v, "Loading...") {
		t.Errorf("view lacks %q:\n%s", "Loading...", v)
	}
}

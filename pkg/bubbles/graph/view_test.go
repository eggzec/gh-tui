package graph

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// octopus is a merge of four branches, one more than a cap of 3 lanes shows.
func octopus() []Commit {
	return []Commit{
		{ID: "o", Parents: []string{"a", "b", "c", "d"}, Short: "0c7095", Title: "Merge branches b, c and d", Right: "1h"},
		{ID: "d", Parents: []string{"a"}, Short: "d4d4d4", Title: "d: docs", Right: "2h"},
		{ID: "c", Parents: []string{"a"}, Short: "c3c3c3", Title: "c: config", Right: "3h"},
		{ID: "b", Parents: []string{"a"}, Short: "b2b2b2", Title: "b: build", Right: "4h"},
		{ID: "a", Short: "a1a1a1", Title: "a: start", Right: "5h"},
	}
}

func TestView(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model
	}{
		{"loading", func(*testing.T) Model {
			return New(newSource(sample(), 10).fetch, WithSize(60, 3), WithFocused(true))
		}},
		{"loaded", func(t *testing.T) Model {
			t.Helper()
			return load(t, newSource(sample(), 10), WithSize(60, 9))
		}},
		{"blurred", func(t *testing.T) Model {
			t.Helper()
			m, _ := keys(t, load(t, newSource(sample(), 10), WithSize(60, 9)), "j", "j")
			m.Blur()
			return m
		}},
		{"40 columns", func(t *testing.T) Model {
			t.Helper()
			m, _ := keys(t, load(t, newSource(sample(), 10), WithSize(40, 9)), "j")
			return m
		}},
		{"right column dropped when narrow", func(t *testing.T) Model {
			t.Helper()
			return load(t, newSource(sample(), 10), WithSize(20, 9))
		}},
		{"lanes over the cap", func(t *testing.T) Model {
			t.Helper()
			return load(t, newSource(octopus(), 10), WithSize(40, 6), WithMaxLanes(3))
		}},
		{"loading more", func(t *testing.T) Model {
			t.Helper()
			m := load(t, newSource(history(40), 10), WithSize(50, 6))
			// Leave the fetch of the next chunk in flight.
			m, _ = m.Update(press("G"))
			return m
		}},
		{"error loading more", func(t *testing.T) Model {
			t.Helper()
			src := newSource(history(40), 10)
			src.setFail("10", errors.New("GET /repos/o/r/commits: 502 Bad Gateway"))
			m, _ := keys(t, load(t, src, WithSize(60, 6)), "G")
			return m
		}},
		{"error", func(t *testing.T) Model {
			t.Helper()
			src := newSource(sample(), 10)
			src.setFail("", errors.New("API rate limit exceeded"))
			return load(t, src, WithSize(60, 3))
		}},
		{"empty", func(t *testing.T) Model {
			t.Helper()
			return load(t, newSource(nil, 10), WithSize(40, 3))
		}},
		{"scrolled", func(t *testing.T) Model {
			t.Helper()
			m, _ := keys(t, load(t, newSource(history(100), 20), WithSize(50, 8)), "pgdown", "pgdown", "k")
			return m
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

// TestViewPlain keeps the sample graph without styles, so the glyphs are
// easy to review.
func TestViewPlain(t *testing.T) {
	m := load(t, newSource(sample(), 10), WithSize(60, 9))
	golden.RequireEqual(t, ansi.Strip(m.View()))
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
	m := load(t, newSource(octopus(), 10), WithSize(40, 6))
	for _, size := range [][2]int{{1, 1}, {2, 3}, {3, 1}, {7, 4}, {9, 2}, {12, 5}, {80, 40}, {200, 2}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m.SetSize(size[0], size[1])
			assertFits(t, m.View(), size[0], size[1])
		})
	}
	m.SetSize(0, 10)
	if m.View() != "" {
		t.Fatal("zero width should render nothing")
	}
}

// The cursor, the commits, the lines of the lanes and the ellipsis of cut
// titles and hidden lanes are the glyphs of the styles, so a view with
// ASCII glyphs draws ASCII alone.
func TestViewGlyphs(t *testing.T) {
	st := DefaultStyles(true)
	st.CursorGlyph, st.CommitGlyph, st.Lines, st.Ellipsis = ">", "*", lipgloss.ASCIIBorder(), "..."
	st.ErrorGlyph, st.ErrorSeparator, st.ErrorEllipsis = "x", " - ", "..."
	for _, m := range []Model{
		load(t, newSource(sample(), 10), WithSize(40, 9), WithStyles(st)),
		load(t, newSource(octopus(), 10), WithSize(30, 6), WithMaxLanes(3), WithStyles(st)),
	} {
		v := ansi.Strip(m.View())
		for _, want := range []string{"> *", "|", "+-", "..."} {
			if !strings.Contains(v, want) {
				t.Errorf("view lacks %q:\n%s", want, v)
			}
		}
		if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("view has glyphs beyond ASCII:\n%s", v)
		}
	}
	loading := New(newSource(sample(), 10).fetch, WithSize(60, 3), WithStyles(st))
	if v := ansi.Strip(loading.View()); !strings.Contains(v, "Loading...") {
		t.Errorf("view lacks %q:\n%s", "Loading...", v)
	}
}

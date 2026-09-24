package calendar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model
	}{
		{"40 columns", func(*testing.T) Model {
			return New(WithWeeks(year(today)), WithSize(40, 10))
		}},
		{"80 columns", func(*testing.T) Model {
			return New(WithWeeks(year(today)), WithSize(80, 10), WithTotal(1234))
		}},
		{"120 columns", func(*testing.T) Model {
			return New(WithWeeks(year(today)), WithSize(120, 10))
		}},
		{"focused", func(t *testing.T) Model {
			t.Helper()
			m := keys(t, New(WithWeeks(year(today)), WithSize(80, 10), WithFocused(true)), "h", "k", "k")
			return m
		}},
		{"narrow", func(*testing.T) Model {
			return New(WithWeeks(year(today)), WithSize(16, 10))
		}},
		{"short and tall", func(*testing.T) Model {
			return New(WithWeeks(year(today)), WithSize(60, 12))
		}},
		{"glyph", func(*testing.T) Model {
			return New(WithWeeks(year(today)), WithSize(40, 10), WithGlyph("#"))
		}},
		{"light", func(*testing.T) Model {
			return New(WithWeeks(year(today)), WithSize(40, 10), WithStyles(DefaultStyles(false)))
		}},
		{"empty", func(*testing.T) Model {
			return New(WithSize(40, 3))
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

// TestViewPlain keeps the calendar without styles, so the layout is easy to
// review.
func TestViewPlain(t *testing.T) {
	m := keys(t, New(WithWeeks(year(today)), WithSize(80, 10), WithFocused(true)), "k")
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
	for _, focused := range []bool{false, true} {
		m := New(WithWeeks(year(today)), WithFocused(focused))
		for _, size := range [][2]int{{1, 1}, {2, 3}, {3, 1}, {7, 4}, {17, 10}, {19, 9}, {33, 10}, {80, 40}, {200, 2}} {
			t.Run(fmt.Sprintf("%dx%d focused %v", size[0], size[1], focused), func(t *testing.T) {
				m.SetSize(size[0], size[1])
				assertFits(t, m.View(), size[0], size[1])
			})
		}
		m.SetSize(0, 10)
		if m.View() != "" {
			t.Fatal("zero width should render nothing")
		}
	}
}

package issues

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		section       func(t *testing.T, width, height int) *host
	}{
		{"list 80", 80, 12, list},
		{"list 120", 120, 12, list},
		{"list 60", 60, 8, list},
		{"closed", 80, 6, func(t *testing.T, width, height int) *host {
			t.Helper()
			s := list(t, width, height)
			press(t, s, "f")
			return s
		}},
		{"no repo", 80, 9, func(t *testing.T, width, height int) *host {
			t.Helper()
			s := newSection(t, newFakeService(nil), width, height)
			run(t, s, s.Init())
			return s
		}},
		{"no issues", 80, 4, func(t *testing.T, width, height int) *host {
			t.Helper()
			return started(t, newFakeService(nil), width, height)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.section(t, tt.width, tt.height)
			v := s.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// list returns a section showing sample issues with the second selected.
func list(t *testing.T, width, height int) *host {
	t.Helper()
	s := started(t, newFakeService(sampleIssues(40)), width, height)
	press(t, s, "down")
	return s
}

func TestViewFitsAnySize(t *testing.T) {
	s := started(t, newFakeService(sampleIssues(40)), 80, 10)
	for _, size := range [][2]int{{1, 1}, {10, 2}, {30, 5}, {45, 5}, {200, 3}} {
		s.SetSize(size[0], size[1])
		run(t, s, s.Update(nil))
		assertFits(t, s.View(), size[0], size[1])
	}
}

// assertFits checks that v is exactly height lines of exactly width cells.
func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, ansi.Strip(v))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Fatalf("line %d is %d cells wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

package repos

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		section       func(t *testing.T, width, height int) *Section
	}{
		{"80 columns", 80, 10, listed},
		{"120 columns", 120, 10, listed},
		{"empty", 80, 5, func(t *testing.T, width, height int) *Section {
			t.Helper()
			return newSection(t, newFake(), width, height)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.section(t, tt.width, tt.height).View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// listed shows the sample repos after two pinned ones, with one current and
// another selected.
func listed(t *testing.T, width, height int) *Section {
	t.Helper()
	f := newFake(sampleRepos()...)
	f.extra = []core.Repo{{
		Ref: ref("golang/go"), Description: "The Go programming language", Language: "Go",
		Stars: 131_000, Starred: true, UpdatedAt: testNow.Add(-20 * time.Minute),
	}}
	s := newSection(t, f, width, height,
		WithPinned([]core.RepoRef{ref("golang/go"), ref("eggzec/gh-tui")}),
		WithCurrent(ref("charmbracelet/bubbletea")),
	)
	keys(s, "down", "down", "down")
	return s
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
			tb.Errorf("line %d is %d cells wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

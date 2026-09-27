package tree

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// benchTree returns a tree with a 40-row window over 10,101 visible rows: one
// directory holding 100 directories of 100 files each, all expanded, with
// the cursor in the middle. Every file shows its size as a detail.
func benchTree(b *testing.B, opts ...Option) Model {
	b.Helper()
	f := newFiles()
	for d := range 100 {
		for i := range 100 {
			id := fmt.Sprintf("repo/dir%03d/file%03d.go", d, i)
			f.add(id)
			f.setDetail(id, "1.2K")
		}
	}
	m := load(b, f, append([]Option{WithSize(100, 40), WithExpandAllLimits(20_000, 2)}, opts...)...)
	m = keys(b, m, "*")
	if m.Len() != 10_101 {
		b.Fatalf("Len() = %d, want 10101", m.Len())
	}
	m.sel = 5_000
	m.scroll()
	return m
}

func BenchmarkView(b *testing.B) {
	m := benchTree(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkViewIcons draws a styled icon before every name, as a file
// browser does.
func BenchmarkViewIcons(b *testing.B) {
	file := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	dir := lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	m := benchTree(b, WithIcons(func(n Node, _ bool) string {
		if n.Branch {
			return dir.Render("d")
		}
		return file.Render("f")
	}))
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := benchTree(b)
	// Box the keys once so the benchmark measures the tree, not the boxing.
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a full window down and back so the window scrolls.
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		m, _ = m.Update(msg)
	}
}

func BenchmarkToggle(b *testing.B) {
	m := benchTree(b)
	// The top directory holds every other row.
	m.sel = 0
	m.scroll()
	enter := tea.Msg(press("enter"))
	b.ReportAllocs()
	for b.Loop() {
		// Expanding the loaded directory again lists all 10,101 rows.
		m, _ = m.Update(enter)
	}
}

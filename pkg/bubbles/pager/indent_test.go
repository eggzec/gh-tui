package pager

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// indentRows is plain rows that each have an indent of their own.
type indentRows struct {
	lines []string
	ind   []int
}

func (r indentRows) Len() int                   { return len(r.lines) }
func (r indentRows) Text(i int) string          { return r.lines[i] }
func (r indentRows) Marks(int) []termtext.Style { return nil }
func (r indentRows) Indent(i int) int           { return r.ind[i] }

// plainRows is the same rows with no Indenter: its Indent shadows the
// embedded method with another signature, so the interface is not met.
type plainRows struct{ indentRows }

func (plainRows) Indent() {}

func screen(m Model) []string {
	return strings.Split(ansi.Strip(m.View()), "\n")
}

func TestIndentNarrowsWrap(t *testing.T) {
	// 20 columns, no gutter text beyond the numbers hidden by a custom gutter.
	r := indentRows{
		lines: []string{strings.Repeat("a", 18), strings.Repeat("b", 18), "tail"},
		ind:   []int{0, 4, 8},
	}
	m := fresh(t, WithSize(20, 12), WithWrap(true), WithRows(r), WithGutter(noGutter{}))
	m.Focus()
	for i, want := range []int{1, 2, 1} {
		if n := m.rowsIn(i); n != want {
			t.Errorf("row %d takes %d rows, want %d", i, n, want)
		}
	}
	got := screen(m)
	exp := []string{
		strings.Repeat("a", 18),
		"    " + strings.Repeat("b", 16),
		"    bb",
		"        tail",
	}
	for i, w := range exp {
		if g := strings.TrimRight(got[i], " "); g != w {
			t.Errorf("screen row %d = %q, want %q", i, g, w)
		}
	}
}

func TestIndentWrapRowsAndPosition(t *testing.T) {
	r := indentRows{
		lines: []string{strings.Repeat("x", 30), strings.Repeat("y", 30), "end"},
		ind:   []int{0, 10, 0},
	}
	m := fresh(t, WithSize(20, 4), WithWrap(true), WithRows(r), WithGutter(noGutter{}))
	m.Focus()
	// 30 cells wrap to 2 rows at 20 columns and to 3 at 10.
	if a, b := m.rowsIn(0), m.rowsIn(1); a != 2 || b != 3 {
		t.Fatalf("rows = %d, %d, want 2, 3", a, b)
	}
	// Byte 25 of the indented row lies on its third row.
	if got := m.rowOf(1, 25); got != 2 {
		t.Errorf("rowOf = %d, want 2", got)
	}
	if got := m.rowOf(0, 25); got != 1 {
		t.Errorf("rowOf unindented = %d, want 1", got)
	}
	m.down(3)
	if m.top != 1 || m.row != 1 {
		t.Errorf("after down(3) at %d row %d, want 1 row 1", m.top, m.row)
	}
	lines := screen(m)
	if g, w := strings.TrimRight(lines[0], " "), "          "+strings.Repeat("y", 10); g != w {
		t.Errorf("first row = %q, want %q", g, w)
	}
	m.up(3)
	if m.top != 0 || m.row != 0 {
		t.Errorf("after up(3) at %d row %d, want 0 row 0", m.top, m.row)
	}
}

func TestIndentScrollsSideways(t *testing.T) {
	r := indentRows{lines: []string{strings.Repeat("z", 40)}, ind: []int{6}}
	m := fresh(t, WithSize(20, 4), WithRows(r), WithGutter(noGutter{}))
	m.Focus()
	m.scrollRight(100)
	// 40 cells, 14 shown right of the indent: 26 is as far as it goes.
	if m.left != 26 {
		t.Errorf("left = %d, want 26", m.left)
	}
	if got := strings.TrimRight(screen(m)[0], " "); got != "      "+strings.Repeat("z", 14) {
		t.Errorf("row 0 = %q", got)
	}
}

func TestNoIndenterIsUnchanged(t *testing.T) {
	lines := []string{strings.Repeat("a", 30), "b"}
	zero := indentRows{lines: lines, ind: []int{0, 0}}
	a := fresh(t, WithSize(20, 6), WithWrap(true), WithRows(zero), WithGutter(noGutter{}))
	b := fresh(t, WithSize(20, 6), WithWrap(true), WithRows(plainRows{indentRows: zero}), WithGutter(noGutter{}))
	a.Focus()
	b.Focus()
	if a.View() != b.View() {
		t.Errorf("a zero indent draws differently from none:\n%s\n%s", a.View(), b.View())
	}
	if n := b.rowsIn(0); n != 2 {
		t.Errorf("rows = %d, want 2", n)
	}
}

func TestIndentWiderThanPager(t *testing.T) {
	r := indentRows{
		lines: []string{strings.Repeat("w", 25), "short"},
		ind:   []int{50, 20},
	}
	m := fresh(t, WithSize(20, 12), WithWrap(true), WithRows(r), WithGutter(noGutter{}))
	m.Focus()
	// One column is left for the text, so each cell takes a row.
	if a, b := m.rowsIn(0), m.rowsIn(1); a != 25 || b != 5 {
		t.Errorf("rows = %d, %d, want 25, 5", a, b)
	}
	lines := screen(m)
	if len(lines) != 12 {
		t.Fatalf("%d lines, want 12", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d is %d cells, want 20", i, w)
		}
	}
	if got, want := strings.TrimRight(lines[0], " "), strings.Repeat(" ", 19)+"w"; got != want {
		t.Errorf("line 0 = %q, want %q", got, want)
	}
}

// noGutter is a gutter that takes no columns.
type noGutter struct{}

func (noGutter) Width(int) int                     { return 0 }
func (noGutter) Write(*strings.Builder, int, bool) {}

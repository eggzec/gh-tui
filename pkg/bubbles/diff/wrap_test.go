package diff

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// longPatch has a context line, an added line that wraps over three rows at
// 40 cells, and more short lines below it.
func longPatch() *source {
	long := strings.Repeat("alpha beta gamma delta ", 4) + "wordacross edge here"
	return &source{size: 5, files: []File{{
		Path: "a.go", Status: StatusModified, Additions: 2, Deletions: 1,
		Patch: "@@ -1,4 +1,5 @@\n ctx one\n-old\n+" + long + "\n+" + long + "\n ctx two\n ctx three\n ctx four",
	}}}
}

func TestWrapView(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model
	}{
		{"long_added", func(t *testing.T) Model {
			t.Helper()
			return load(t, longPatch(), WithSize(40, 14), WithWrap(true))
		}},
		{"cursor_on_wrapped", func(t *testing.T) Model {
			t.Helper()
			m := load(t, longPatch(), WithSize(40, 14), WithWrap(true))
			return keys(t, m, "down", "down", "down", "down")
		}},
		{"search_across_segment", func(t *testing.T) Model {
			t.Helper()
			m := load(t, longPatch(), WithSize(40, 14), WithWrap(true))
			return keys(t, m, "/", "w", "o", "r", "d", "a", "c", "r", "o", "s", "s", "enter")
		}},
		{"page_down", func(t *testing.T) Model {
			t.Helper()
			m := load(t, longPatch(), WithSize(40, 8), WithWrap(true))
			return keys(t, m, "ctrl+f")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := tt.model(t)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			golden.RequireEqual(t, ansi.Strip(v))
		})
	}
}

func TestWrapSettings(t *testing.T) {
	t.Parallel()
	m := load(t, longPatch(), WithSize(40, 14))
	if m.Wrap() {
		t.Fatal("wrap is on by default")
	}
	m = keys(t, m, "right")
	if m.left == 0 {
		t.Fatal("an unwrapped view scrolls sideways")
	}
	m.SetWrap(true)
	if !m.Wrap() || m.left != 0 {
		t.Errorf("SetWrap(true): wrap %v, left %d", m.Wrap(), m.left)
	}
	if m.maxLeft() != 0 {
		t.Error("a wrapped view scrolls sideways")
	}
	m = keys(t, m, "right", "l")
	if m.left != 0 {
		t.Errorf("right moved a wrapped view sideways: left %d", m.left)
	}
	m.SetWrap(false)
	if m.Wrap() || m.sub != 0 {
		t.Errorf("SetWrap(false): wrap %v, sub %d", m.Wrap(), m.sub)
	}
	m = keys(t, m, "right")
	if m.left == 0 {
		t.Error("right does not scroll sideways again once wrap is off")
	}
}

// TestWrapOffUnchanged checks that turning wrap on and off again leaves the
// view as a view that never wrapped.
func TestWrapOffUnchanged(t *testing.T) {
	t.Parallel()
	want := load(t, longPatch(), WithSize(40, 14)).View()
	m := load(t, longPatch(), WithSize(40, 14))
	m.SetWrap(true)
	m.SetWrap(false)
	if got := m.View(); got != want {
		t.Errorf("view after wrap on and off differs:\n%s\nwant:\n%s", got, want)
	}
	if got := load(t, longPatch(), WithSize(40, 14), WithWrap(false)).View(); got != want {
		t.Errorf("WithWrap(false) differs from the default:\n%s", got)
	}
}

// tall is the first line of m that wraps over more than one row.
func tall(t *testing.T, m Model) int {
	t.Helper()
	for r := range m.layout.Len() {
		if m.rowsIn(r, m.numWidth()) > 1 {
			return r
		}
	}
	t.Fatal("no line wraps")
	return 0
}

// cursorRows counts the rows of v that carry the cursor's mark.
func cursorRows(v string) int {
	n := 0
	for l := range strings.SplitSeq(ansi.Strip(v), "\n") {
		if strings.HasPrefix(l, "▌") {
			n++
		}
	}
	return n
}

func TestWrapResizeClampsSub(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		width, height int
		wantSub       int
	}{
		{"wider, one row", 160, 14, 0},
		{"narrower, more rows", 30, 14, -1},
		{"same size", 40, 14, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := load(t, longPatch(), WithSize(40, 14), WithWrap(true))
			l := tall(t, m)
			m.cursor, m.top, m.sub = l+1, l, 3
			m.SetSize(tt.width, tt.height)
			if m.top < 0 || m.top >= m.layout.Len() {
				t.Fatalf("top %d is out of range", m.top)
			}
			if rows := m.rowsIn(m.top, m.numWidth()); m.sub < 0 || m.sub >= rows {
				t.Errorf("sub %d is outside the %d rows of line %d", m.sub, rows, m.top)
			}
			if tt.wantSub >= 0 && m.sub != tt.wantSub {
				t.Errorf("sub = %d, want %d", m.sub, tt.wantSub)
			}
			assertFits(t, m.View(), tt.width, tt.height)
		})
	}
}

func TestWrapLineSteps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		height int
		keys   []string
		// after, relative to the first wrapped line
		wantCursor int
		wantTop    int // -1 for any
	}{
		{"down steps a line, not a row", 14, []string{"j"}, 1, -1},
		{"up steps a line", 14, []string{"j", "j", "k"}, 1, -1},
		{"line taller than the body shows from its first row", 5, []string{"j", "j"}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := load(t, longPatch(), WithSize(40, tt.height), WithWrap(true))
			l := tall(t, m)
			m.cursor = l
			m.scroll()
			if tt.wantTop == 0 {
				m = keys(t, m, tt.keys[:0]...)
				if m.cursor != l || m.sub != 0 || m.top > l {
					t.Fatalf("cursor %d top %d sub %d: the tall line does not start the cursor's rows", m.cursor, m.top, m.sub)
				}
				if got := cursorRows(m.View()); got != m.bodyHeight()-1 && got != m.bodyHeight() {
					t.Errorf("a line taller than the body shows %d rows of the cursor, body %d", got, m.bodyHeight())
				}
				return
			}
			m = keys(t, m, tt.keys...)
			if m.cursor != l+tt.wantCursor {
				t.Errorf("cursor = %d, want %d", m.cursor, l+tt.wantCursor)
			}
		})
	}
}

func TestWrapPaging(t *testing.T) {
	t.Parallel()
	// by is the line k visual rows from the cursor, as the walk of the lines
	// counts them.
	by := func(m Model, k int) int {
		numW, n := m.numWidth(), m.layout.Len()
		acc := 0
		if k >= 0 {
			for r := m.cursor; r < n-1; r++ {
				rows := m.rowsIn(r, numW)
				if acc+rows > k {
					return r
				}
				acc += rows
			}
			return n - 1
		}
		for r := m.cursor; r > 0; r-- {
			acc += m.rowsIn(r-1, numW)
			if acc >= -k {
				return r - 1
			}
		}
		return 0
	}
	tests := []struct {
		name string
		pre  []string
		key  string
		step func(h int) int
	}{
		{"page down", nil, "ctrl+f", func(h int) int { return h }},
		{"half page down", nil, "ctrl+d", func(h int) int { return max(h/2, 1) }},
		{"page up", []string{"G"}, "ctrl+b", func(h int) int { return -h }},
		{"half page up", []string{"G"}, "ctrl+u", func(h int) int { return -max(h/2, 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := keys(t, load(t, longPatch(), WithSize(40, 8), WithWrap(true)), tt.pre...)
			want := by(m, tt.step(m.bodyHeight()))
			m = keys(t, m, tt.key)
			if m.cursor != want {
				t.Errorf("cursor = %d, want %d", m.cursor, want)
			}
			if got := cursorRows(m.View()); got == 0 {
				t.Error("the cursor is not in view")
			}
		})
	}
}

func TestWrapSearchCentres(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
	}{
		{"a line below the window", "ctx four"},
		{"the wrapped line", "wordacross"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := load(t, longPatch(), WithSize(40, 8), WithWrap(true))
			ks := []string{"/"}
			for _, r := range tt.query {
				if r == ' ' {
					ks = append(ks, "space")
					continue
				}
				ks = append(ks, string(r))
			}
			m = keys(t, m, append(ks, "enter")...)
			if !m.inView(m.cursor, m.bodyHeight()) && m.rowsIn(m.cursor, m.numWidth()) < m.bodyHeight() {
				t.Errorf("the match on line %d is not in the window (top %d, sub %d)", m.cursor, m.top, m.sub)
			}
			assertFits(t, m.View(), 40, 8)
		})
	}
}

func twoFiles() *source {
	return &source{size: 5, files: []File{
		longPatch().files[0],
		goFile("b.go", 1),
	}}
}

func TestWrapFileEnd(t *testing.T) {
	t.Parallel()
	m := load(t, twoFiles(), WithSize(40, 8), WithWrap(true))
	if m.FileEndVisible() {
		t.Fatal("the end of a file of 13 rows shows in a window of 7")
	}
	for range 10 {
		if m.FileEndVisible() {
			break
		}
		cmd := m.PageDownWithinFile()
		m = run(t, m, cmd)
		if f, _ := m.layout.FileAt(m.cursor); f != 0 {
			t.Fatalf("PageDownWithinFile left the file with its end out of view: cursor %d", m.cursor)
		}
	}
	if !m.FileEndVisible() {
		t.Fatal("paging never showed the end of the file")
	}
	m = run(t, m, m.PageDownWithinFile())
	if f, _ := m.layout.FileAt(m.cursor); f != 1 {
		t.Errorf("with the end in view, paging on stays in file %d", f)
	}
}

func TestWrapWideAndANSI(t *testing.T) {
	t.Parallel()
	const wide = 40
	src := func() *source {
		return &source{size: 5, files: []File{{
			Path: "a.go", Status: StatusModified, Additions: 2,
			Patch: "@@ -1,1 +1,3 @@\n ctx\n+" + strings.Repeat("日本語", wide) + "\n+\x1b[31mred\x1b[0m " + strings.Repeat("q", 100),
		}}}
	}
	for width := 30; width <= 45; width++ {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			t.Parallel()
			m := load(t, src(), WithSize(width, 80), WithWrap(true))
			v := m.View()
			assertFits(t, v, width, 80)
			plain := ansi.Strip(v)
			if got := strings.Count(plain, "日"); got != wide {
				t.Errorf("%d of %d wide runes shown", got, wide)
			}
			if got := strings.Count(plain, "q"); got != 100 {
				t.Errorf("%d of 100 cells of text shown after the escape", got)
			}
			if !strings.Contains(plain, "red") || strings.Contains(v, "\x1b[31mred") {
				t.Errorf("an escape of the text is drawn, or its text is lost")
			}
		})
	}
}

// TestWrapStylesCarry checks that a search hit, the cursor's mark and a
// selection are drawn on each visual row of a line, as the view without them
// differs from the view with them there.
func TestWrapStylesCarry(t *testing.T) {
	t.Parallel()
	rowsOf := func(v string) []string { return strings.Split(v, "\n") }
	tests := []struct {
		name  string
		build func(t *testing.T) (styled, plain Model)
		rows  []int
	}{
		{"search hit across a segment edge", func(t *testing.T) (Model, Model) {
			t.Helper()
			m := keys(t, load(t, longPatch(), WithSize(40, 14), WithWrap(true)),
				"/", "w", "o", "r", "d", "a", "c", "r", "o", "s", "s", "enter")
			p := load(t, longPatch(), WithSize(40, 14), WithWrap(true))
			p.cursor, p.top, p.sub = m.cursor, m.top, m.sub
			return m, p
		}, []int{6, 7, 10, 11}},
		{"cursor on every row", func(t *testing.T) (Model, Model) {
			t.Helper()
			m := load(t, longPatch(), WithSize(40, 14), WithWrap(true))
			p := m
			m.cursor = tall(t, m)
			return m, p
		}, []int{4, 5, 6, 7}},
		{"selection reaching the cursor line", func(t *testing.T) (Model, Model) {
			t.Helper()
			// longPatch's hunk header miscounts its lines, so it shows as raw
			// text, which a selection leaves alone: this one counts them right.
			src := longPatch()
			src.files[0].Patch = strings.Replace(src.files[0].Patch, "@@ -1,4 +1,5 @@", "@@ -1,5 +1,6 @@", 1)
			p := load(t, src, WithSize(40, 14), WithWrap(true))
			p.cursor = tall(t, p)
			m := p
			m.SetAnchor(Pos{Path: "a.go", Side: NewSide, Line: 1})
			if lo, hi := m.selection(); lo >= hi {
				t.Fatalf("selection is rows %d to %d", lo, hi)
			}
			return m, p
		}, []int{4, 5, 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			styled, plain := tt.build(t)
			a, b := rowsOf(styled.View()), rowsOf(plain.View())
			for _, r := range tt.rows {
				if a[r] == b[r] {
					t.Errorf("row %d is drawn the same with and without the style: %q", r, a[r])
				}
			}
		})
	}
}

// TestWrapCursorLineShows checks that the whole cursor line is in the window,
// below the sticky header, wherever the cursor is, over a diff whose second
// file has wider line numbers and lines that wrap at some widths.
func TestWrapCursorLineShows(t *testing.T) {
	t.Parallel()
	src := func() *source {
		var b strings.Builder
		b.WriteString("@@ -98,6 +98,6 @@")
		for i := range 6 {
			b.WriteString("\n+")
			b.WriteString(strings.Repeat("y", 28+i*2))
		}
		return &source{size: 5, files: []File{
			longPatch().files[0],
			{Path: "b.go", Status: StatusModified, Additions: 6, Patch: b.String()},
		}}
	}
	for width := 34; width <= 46; width++ {
		for height := 6; height <= 10; height++ {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				t.Parallel()
				m := load(t, src(), WithSize(width, height), WithWrap(true))
				for range m.layout.Len() + 1 {
					v := m.View()
					assertFits(t, v, width, height)
					want := min(m.rowsIn(m.cursor, m.numWidth()), m.bodyHeight()-1)
					if got := cursorRows(v); got < want {
						t.Fatalf("cursor %d (top %d, sub %d): %d of %d rows shown\n%s", m.cursor, m.top, m.sub, got, want, ansi.Strip(v))
					}
					m = keys(t, m, "j")
				}
			})
		}
	}
}

// TestWrapStickyKeepsCursorRows checks that the sticky header never covers a
// row of the cursor's line.
func TestWrapStickyKeepsCursorRows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		cursorOff  int
		topOff     int
		sub        int
		wantCursor int
	}{
		{"cursor line at the top", 0, 0, 0, 4},
		{"window inside the cursor line", 0, 0, 2, 4},
		{"window inside the line above", 1, 0, 3, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := load(t, longPatch(), WithSize(40, 8), WithWrap(true))
			l := tall(t, m)
			m.cursor, m.top, m.sub = l+tt.cursorOff, l+tt.topOff, tt.sub
			m.scroll()
			v := m.View()
			assertFits(t, v, 40, 8)
			if got := cursorRows(v); got != tt.wantCursor {
				t.Errorf("%d rows of the cursor line shown, want %d (top %d, sub %d)\n%s", got, tt.wantCursor, m.top, m.sub, ansi.Strip(v))
			}
		})
	}
}

// TestWrapSubSurvivesChanges checks the window of a wrapped view when rows
// move around it: it keeps its first visual row while it starts on the same
// line, and starts on a line's first row once the line it was on has moved
// from under it.
func TestWrapSubSurvivesChanges(t *testing.T) {
	t.Parallel()
	// b.go is short and comes first, so a.go's long lines are rows 10 and 11.
	// longPatch's hunk header miscounts its lines, so a.go gets one that counts
	// them right, as a diff of lines and not raw text.
	long := strings.Repeat("alpha beta gamma delta ", 4) + "wordacross edge here"
	whole := "ctx one\n" + long + "\n" + long + "\nctx two\nctx three\nctx four\n"
	src := func() *source {
		a := longPatch().files[0]
		a.Patch = strings.Replace(a.Patch, "@@ -1,4 +1,5 @@", "@@ -1,5 +1,6 @@", 1)
		return &source{size: 5, files: []File{goFile("b.go", 1), a}}
	}
	setup := func(t *testing.T) Model {
		t.Helper()
		m := load(t, src(), WithSize(40, 10), WithWrap(true))
		m.top, m.sub, m.cursor = 10, 1, 11
		m.scroll()
		if m.top != 10 || m.sub != 1 || m.rowsIn(10, m.numWidth()) < 3 {
			t.Fatalf("window starts at top %d, sub %d", m.top, m.sub)
		}
		return m
	}
	topText := func(m Model) string {
		r, _ := m.layout.RowAt(m.top)
		return r.Text
	}
	tests := []struct {
		name    string
		change  func(t *testing.T, m *Model)
		sameTop bool
	}{
		{"block below the cursor", func(_ *testing.T, m *Model) { m.SetBlocks([]Block{at("n", "a.go", 4)}) }, true},
		{"block above the window", func(_ *testing.T, m *Model) { m.SetBlocks([]Block{at("n", "a.go", 1)}) }, true},
		{"block between the top and the cursor", func(_ *testing.T, m *Model) { m.SetBlocks([]Block{at("n", "a.go", 2)}) }, false},
		{"file above shown whole", func(t *testing.T, m *Model) {
			t.Helper()
			if !m.SetFull("b.go", numbered(30, nil)) {
				t.Fatal("SetFull refused")
			}
		}, true},
		{"file of the cursor shown whole", func(t *testing.T, m *Model) {
			t.Helper()
			if !m.SetFull("a.go", whole) {
				t.Fatal("SetFull refused")
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := setup(t)
			was := topText(m)
			tt.change(t, &m)
			m = settle(t, m)
			if tt.sameTop {
				if topText(m) != was || m.sub != 1 {
					t.Errorf("window at %q sub %d, want %q sub 1", topText(m), m.sub, was)
				}
			} else if m.sub != 0 {
				t.Errorf("the window moved to another line and kept sub %d", m.sub)
			}
			if rows := m.rowsIn(m.top, m.numWidth()); m.sub >= rows {
				t.Errorf("sub %d is not within the %d rows of line %d", m.sub, rows, m.top)
			}
			assertFits(t, m.View(), 40, 10)
		})
	}
}

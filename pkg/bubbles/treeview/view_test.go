package treeview

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// thread is a folded or open row that hides replies, as the conversation
// has them.
func thread(id string, closed bool, marker string) Node {
	r := fold(id, 0)
	r.Closed = closed
	r.Marker = marker
	return r
}

func TestViewGuides(t *testing.T) {
	m := newTree(files(), WithSize(30, 9))
	v := m.View()
	assertFits(t, v, 30, 9)
	golden.RequireEqual(t, plain(v))
}

func TestViewBodyLinesKeepTheGuide(t *testing.T) {
	rows := []Node{
		open(fold("Review", 0)),
		{ID: "t1", Depth: 1, Branch: true, Lines: []string{"alice  src/main.go:12", "the first line of a comment", "the second line"}},
		{ID: "t2", Depth: 1, Lines: []string{"bob  a last thread", "with a body", "of two lines"}},
		{ID: "t3", Depth: 0, Lines: []string{"summary", "of a top-level row"}},
	}
	rows[1].Closed = false
	m := newTree(rows, WithSize(34, 10))
	v := m.View()
	assertFits(t, v, 34, 10)
	golden.RequireEqual(t, plain(v))
}

func TestViewFoldMarkers(t *testing.T) {
	rows := []Node{
		thread("t1", true, "⋯ 2 replies"),
		thread("t2", false, "⋯ 2 replies"),
		{ID: "d", Branch: true, Closed: true, Marker: "⋯ 3 lines", Lines: []string{"<details> Summary of a block", "line a", "line b", "line c"}},
		{ID: "d2", Branch: true, Marker: "⋯ 3 lines", Lines: []string{"<details> Open block", "line a"}},
		{ID: "r", Branch: true, Closed: true, Marker: "⋯", Right: "2h", Lines: []string{"a run of comments"}},
		{ID: "n", Branch: true, Closed: true, Lines: []string{"folder"}},
		{ID: "o", Branch: true, Lines: []string{"folder"}},
	}
	for i := range rows {
		if rows[i].ID == "t1" || rows[i].ID == "t2" {
			rows[i].Lines = []string{"alice  thread " + rows[i].ID}
		}
	}
	m := newTree(rows, WithSize(40, 9))
	v := m.View()
	assertFits(t, v, 40, 9)
	golden.RequireEqual(t, plain(v))
}

// The marker comes after two spaces, at the end of the text and before the
// right part; a long text is cut to keep it, unless that leaves the text under
// 8 columns, and a long row cuts its text before it drops the right part.
func TestViewRowEnds(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		marker string
		right  string
		closed bool
		width  int
		want   string
	}{
		{"marker after the text", "alice replied", "⋯ 2 replies", "", true, 40, " ▸ alice replied  ⋯ 2 replies"},
		{"no marker while open", "alice replied", "⋯ 2 replies", "", false, 40, " ▾ alice replied"},
		{"marker before the right part", "alice", "⋯ 2 replies", "3d", true, 30, " ▸ alice  ⋯ 2 replies" + strings.Repeat(" ", 8) + "3d"},
		{"text cut to keep the marker", "alice replied to a long thread", "⋯ 2 replies", "", true, 30, " ▸ alice replied …  ⋯ 2 replies"},
		{"marker dropped under 8 columns of text", "alice replied to a long thread", "⋯ 2 replies", "", true, 20, " ▸ alice replied to …"},
		{"marker kept with 10 columns of text", "alice replied to a long thread", "⋯ 2 replies", "", true, 23, " ▸ alice r…  ⋯ 2 replies"},
		{"right part pads", "alice", "", "3d", false, 12, " alice" + strings.Repeat(" ", 5) + "3d"},
		{"text cut before the right part", "alice replied to a thread", "", "edited 3d", false, 24, " alice replied… edited 3d"},
		{"right part dropped when alone it does not fit", "alice", "", "edited 3 days ago", false, 12, " alice"},
		{"right part leaves a column of text", "alice", "", "3d", false, 5, " a… 3d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Node{ID: "r", Lines: []string{tt.text}, Branch: tt.marker != "", Closed: tt.closed, Marker: tt.marker, Right: tt.right}
			m := newTree([]Node{r}, WithSize(tt.width+1, 1), WithFocused(false))
			// The cursor column is a space in the plain view, and the cursor glyph
			// is not part of what this checks.
			got := strings.TrimRight(plain(m.View()), " ")
			got = " " + strings.TrimPrefix(got, "▌")
			if want := strings.TrimRight(tt.want, " "); got != want {
				t.Errorf("row = %q, want %q", got, want)
			}
		})
	}
}

func TestViewFoldArrows(t *testing.T) {
	rows := []Node{
		{ID: "a", Branch: true, Closed: true, Lines: []string{"closed"}},
		{ID: "b", Branch: true, Lines: []string{"open"}},
		{ID: "c", Depth: 1, Lines: []string{"child"}},
	}
	got := strings.Split(plain(newTree(rows, WithSize(14, 3), WithFocused(false)).View()), "\n")
	// A leaf keeps the two cells of the arrow, so that siblings line up.
	want := []string{"▌▸ closed", " ▾ open", " ╰─   child"}
	for i := range want {
		if strings.TrimRight(got[i], " ") != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestViewNarrow(t *testing.T) {
	rows := files()
	rows[3].Right = "+5 −2"
	rows[4].Marker = "m"
	for _, w := range []int{1, 2, 3, 5, 8, 12, 20} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			m := newTree(rows, WithSize(w, 9))
			v := m.View()
			assertFits(t, v, w, 9)
			golden.RequireEqual(t, plain(v))
		})
	}
}

func TestViewFitsAnySize(t *testing.T) {
	rows := files()
	rows[0].Marker, rows[0].Right = "⋯ 9", "right"
	rows[1].Lines = []string{"一二三四五六七八九十", "body 一二三"}
	for w := range 25 {
		for h := range 13 {
			m := newTree(rows, WithSize(w, h))
			m, _ = keys(t, m, "j", "j", "G", "h", "*")
			v := m.View()
			if w == 0 || h == 0 {
				if v != "" {
					t.Fatalf("%dx%d view = %q, want none", w, h, v)
				}
				continue
			}
			assertFits(t, v, w, h)
		}
	}
}

// rowBG is the background of the cursor row, #2a2f3a.
const rowBG = "\x1b[48;2;42;47;58m"

func TestViewCursorRow(t *testing.T) {
	rows := []Node{
		leaf("a", 0),
		{ID: "chips", Lines: []string{"label " + lipgloss.NewStyle().Background(lipgloss.Color("#ff0000")).Render("chip") + " after", "body"}},
		leaf("z", 0),
	}
	focused := newTree(rows, WithSize(16, 4))
	focused, _ = keys(t, focused, "j")
	t.Run("focused", func(t *testing.T) {
		v := focused.View()
		assertFits(t, v, 16, 4)
		golden.RequireEqual(t, v)
	})
	t.Run("blurred", func(t *testing.T) {
		blurred := focused
		blurred.Blur()
		v := blurred.View()
		assertFits(t, v, 16, 4)
		golden.RequireEqual(t, v)
		if strings.Contains(v, rowBG) {
			t.Errorf("a blurred tree shades the cursor row: %q", v)
		}
	})
	if v := focused.View(); strings.Count(v, rowBG) < 2*2 {
		t.Errorf("the cursor row has no background after its chip: %q", v)
	}
}

func TestViewNeverReverses(t *testing.T) {
	m := newTree(files(), WithSize(20, 8))
	if v := m.View(); strings.Contains(v, "\x1b[7m") || strings.Contains(v, ";7m") {
		t.Errorf("view reverses video: %q", v)
	}
}

func TestViewEmpty(t *testing.T) {
	m := newTree(nil, WithSize(20, 3))
	v := m.View()
	assertFits(t, v, 20, 3)
	if !strings.HasPrefix(plain(v), "Nothing to show.") {
		t.Errorf("view = %q", plain(v))
	}
}

func TestViewScrolled(t *testing.T) {
	rows := make([]Node, 1, 13)
	rows[0] = open(fold("top", 0))
	for i := range 12 {
		r := leaf(fmt.Sprintf("child %02d", i), 1)
		if i%4 == 0 {
			r.Lines = append(r.Lines, "  a body line")
		}
		rows = append(rows, r)
	}
	m := newTree(rows, WithSize(24, 6))
	m, _ = keys(t, m, "G")
	v := m.View()
	assertFits(t, v, 24, 6)
	golden.RequireEqual(t, plain(v))
}

// deep is a tree of three open folds, one inside the other, with leaves at the
// bottom that are many more than the window shows.
func deep(leaves int) []Node {
	rows := make([]Node, 3, 3+leaves)
	rows[0], rows[1], rows[2] = open(fold("root", 0)), open(fold("sec", 1)), open(fold("sub", 2))
	for i := range leaves {
		rows = append(rows, leaf(fmt.Sprintf("leaf %02d", i), 3))
	}
	return rows
}

// When the tree is scrolled the two nearest ancestors of its top row stay
// pinned above it, with their own guides.
func TestViewPinsTwoAncestors(t *testing.T) {
	m := newTree(deep(12), WithSize(24, 7))
	m, _ = keys(t, m, "G")
	v := m.View()
	assertFits(t, v, 24, 7)
	golden.RequireEqual(t, plain(v))
}

func TestViewPinsNothingAtTheTop(t *testing.T) {
	m := newTree(deep(12), WithSize(24, 7))
	if got, _, _ := strings.Cut(plain(m.View()), "\n"); !strings.Contains(got, "root") {
		t.Errorf("first line = %q, want the first row", got)
	}
	if len(m.pins) != 0 {
		t.Errorf("pins = %v at the top, want none", m.pins)
	}
}

// A cursor that climbs onto an ancestor that was pinned shows it in its place,
// once, and the pins above it are those of its own place.
func TestViewCursorClimbsOntoAPinnedAncestor(t *testing.T) {
	m := newTree(deep(12), WithSize(24, 7))
	m, _ = keys(t, m, "G")
	lines := strings.Split(plain(m.View()), "\n")
	if !strings.Contains(lines[0], "sec") || !strings.Contains(lines[1], "sub") {
		t.Fatalf("pins = %q, %q, want sec and sub", lines[0], lines[1])
	}
	for selected(m) != "sub" {
		m, _ = keys(t, m, "k")
		assertShowsOnce(t, m)
	}
	lines = strings.Split(plain(m.View()), "\n")
	if !strings.Contains(lines[0], "root") || !strings.Contains(lines[1], "sec") {
		t.Errorf("pins over sub = %q, %q, want root and sec", lines[0], lines[1])
	}
	if !strings.Contains(lines[2], "▌") || !strings.Contains(lines[2], "sub") {
		t.Errorf("line 2 = %q, want the cursor on sub", lines[2])
	}
}

// assertShowsOnce checks that the row under the cursor is drawn once, with the
// cursor, whatever the pins are. A row taller than the window may be read
// from its middle, so it only has to show a line with the cursor.
func assertShowsOnce(tb testing.TB, m Model) {
	tb.Helper()
	id := selected(m)
	if m.tall() {
		for l := range strings.SplitSeq(plain(m.View()), "\n") {
			if strings.HasPrefix(l, "▌") {
				return
			}
		}
		tb.Fatalf("no line of the tall row %q has the cursor:\n%s", id, plain(m.View()))
	}
	n, cursor := 0, 0
	for l := range strings.SplitSeq(plain(m.View()), "\n") {
		if strings.HasSuffix(strings.TrimRight(l, " "), id) {
			n++
			if strings.HasPrefix(l, "▌") {
				cursor++
			}
		}
	}
	if n != 1 || cursor != 1 {
		tb.Fatalf("row %q is drawn %d times, %d with the cursor:\n%s", id, n, cursor, plain(m.View()))
	}
}

// Moving down and up the whole tree, at every height, the cursor row is
// always in view, and always drawn once.
func TestViewPinsNeverCoverTheCursor(t *testing.T) {
	for h := 1; h <= 9; h++ {
		m := newTree(deep(8), WithSize(20, h))
		for range 14 {
			m, _ = keys(t, m, "j")
			assertFits(t, m.View(), 20, h)
			assertShowsOnce(t, m)
		}
		for range 14 {
			m, _ = keys(t, m, "k")
			assertFits(t, m.View(), 20, h)
			assertShowsOnce(t, m)
		}
		m, _ = keys(t, m, "G", "ctrl+b", "ctrl+f", "ctrl+u", "ctrl+d")
		assertShowsOnce(t, m)
	}
}

// A tree that is too short for pins keeps a line for rows: none at a height of
// one and the nearest ancestor at a height of two.
func TestViewPinsLeaveALineForRows(t *testing.T) {
	m := newTree(deep(12), WithSize(24, 7))
	m, _ = keys(t, m, "G")
	m.SetSize(24, 2)
	lines := strings.Split(plain(m.View()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "sub") || !strings.Contains(lines[1], "leaf 11") {
		t.Errorf("height 2 = %q, want sub pinned over the cursor row", lines)
	}
	m.SetSize(24, 1)
	assertFits(t, m.View(), 24, 1)
	if got := plain(m.View()); !strings.Contains(got, "leaf 11") {
		t.Errorf("height 1 = %q, want the cursor row", got)
	}
	m.SetSize(24, 7)
	if got, _, _ := strings.Cut(plain(m.View()), "\n"); !strings.Contains(got, "sec") {
		t.Errorf("height 7 again = %q, want sec pinned", got)
	}
}

// Paging counts the lines the pins leave for rows.
func TestPagingStaysBelowThePins(t *testing.T) {
	m := newTree(deep(20), WithSize(24, 8))
	m, _ = keys(t, m, "G", "ctrl+b")
	assertShowsOnce(t, m)
	if got, want := selected(m), "leaf 13"; got != want {
		t.Errorf("a page up from the end = %q, want %q", got, want)
	}
}

// The body of a row carries its guide on to the row's children, at every
// depth, and is cut to what the guide leaves.
func TestViewBodyGuideReachesTheChildren(t *testing.T) {
	rows := []Node{
		{ID: "p", Branch: true, Lines: []string{"head", "body one", "body two"}},
		{ID: "c1", Depth: 1, Lines: []string{"first child"}},
		{ID: "c2", Depth: 1, Branch: true, Lines: []string{"second child", "child body", "a long child body line"}},
		{ID: "g1", Depth: 2, Lines: []string{"grandchild"}},
		{ID: "g2", Depth: 2, Lines: []string{"last grandchild"}},
		{ID: "q", Depth: 0, Branch: true, Lines: []string{"other", "other body"}},
		{ID: "q1", Depth: 1, Lines: []string{"only child"}},
	}
	m := newTree(rows, WithSize(22, 14))
	v := m.View()
	assertFits(t, v, 22, 14)
	golden.RequireEqual(t, plain(v))
}

// Text from outside is not kept by the tree: changing the lines the caller
// handed over leaves the tree as it was.
func TestSetRowsDoesNotAliasTheCallersLines(t *testing.T) {
	lines := []string{"head", "body"}
	m := newTree([]Node{{ID: "a", Lines: lines}}, WithSize(10, 2))
	before := plain(m.View())
	lines[0], lines[1] = "changed", "changed"
	if after := plain(m.View()); after != before {
		t.Errorf("view changed with the caller's lines: %q, was %q", after, before)
	}
}

// Text cut by the width keeps its links closed, and never splits a wide
// character, so the next line starts clean.
func TestViewCutsLinksAndWideText(t *testing.T) {
	link := ansi.SetHyperlink("https://example.test/x") + "a link that is long" + ansi.ResetHyperlink()
	rows := []Node{
		{ID: "l", Lines: []string{"see " + link + " now"}},
		{ID: "w", Lines: []string{"一二三四五六七八九十一二三四五六七八九十"}, Right: "界"},
		{ID: "e", Lines: []string{"日本語 é 🙂🙂🙂 end"}, Marker: "⋯", Branch: true, Closed: true},
	}
	for _, w := range []int{9, 10, 11, 16, 24} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			m := newTree(rows, WithSize(w, 3))
			v := m.View()
			assertFits(t, v, w, 3)
			golden.RequireEqual(t, plain(v))
			for i, l := range strings.Split(v, "\n") {
				opens := strings.Count(l, "\x1b]8;;https")
				closes := strings.Count(l, "\x1b]8;;\x1b\\") + strings.Count(l, "\x1b]8;;\a")
				if opens != closes {
					t.Errorf("line %d opens %d links and closes %d: %q", i, opens, closes, l)
				}
			}
		})
	}
}

func TestTabsExpandToStopsOfFour(t *testing.T) {
	rows := []Node{{ID: "t", Lines: []string{"a\tb\tc", "\tbody", "x\nsplit"}}}
	m := newTree(rows, WithSize(14, 5), WithFocused(false))
	got := strings.Split(plain(m.View()), "\n")
	// A row of several lines is a fold, so it has an arrow.
	want := []string{"▌▾ a   b   c", "▌    body", "▌x", "▌split", ""}
	for i := range want {
		if strings.TrimRight(got[i], " ") != strings.TrimRight(want[i], " ") {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

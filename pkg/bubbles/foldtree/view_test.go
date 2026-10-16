package foldtree

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
func thread(id string, closed bool, marker string) Row {
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
	rows := []Row{
		open(fold("Review", 0)),
		{ID: "t1", Depth: 1, Fold: true, Lines: []string{"alice  src/main.go:12", "the first line of a comment", "the second line"}},
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
	rows := []Row{
		thread("t1", true, "⋯ 2 replies"),
		thread("t2", false, "⋯ 2 replies"),
		{ID: "d", Fold: true, Closed: true, Marker: "⋯ 3 lines", Lines: []string{"<details> Summary of a block", "line a", "line b", "line c"}},
		{ID: "d2", Fold: true, Marker: "⋯ 3 lines", Lines: []string{"<details> Open block", "line a"}},
		{ID: "r", Fold: true, Closed: true, Marker: "⋯", Right: "2h", Lines: []string{"a run of comments"}},
		{ID: "n", Fold: true, Closed: true, Arrow: true, Lines: []string{"folder"}},
		{ID: "o", Fold: true, Arrow: true, Lines: []string{"folder"}},
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
		{"marker after the text", "alice replied", "⋯ 2 replies", "", true, 40, " alice replied  ⋯ 2 replies"},
		{"no marker while open", "alice replied", "⋯ 2 replies", "", false, 40, " alice replied"},
		{"marker before the right part", "alice", "⋯ 2 replies", "3d", true, 30, " alice  ⋯ 2 replies" + strings.Repeat(" ", 10) + "3d"},
		{"text cut to keep the marker", "alice replied to a long thread", "⋯ 2 replies", "", true, 30, " alice replied to…  ⋯ 2 replies"},
		{"marker dropped under 8 columns of text", "alice replied to a long thread", "⋯ 2 replies", "", true, 20, " alice replied to a …"},
		{"marker kept with 10 columns of text", "alice replied to a long thread", "⋯ 2 replies", "", true, 23, " alice rep…  ⋯ 2 replies"},
		{"right part pads", "alice", "", "3d", false, 12, " alice" + strings.Repeat(" ", 5) + "3d"},
		{"text cut before the right part", "alice replied to a thread", "", "edited 3d", false, 24, " alice replied… edited 3d"},
		{"right part dropped when alone it does not fit", "alice", "", "edited 3 days ago", false, 12, " alice"},
		{"right part leaves a column of text", "alice", "", "3d", false, 5, " a… 3d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Row{ID: "r", Lines: []string{tt.text}, Fold: tt.marker != "", Closed: tt.closed, Marker: tt.marker, Right: tt.right}
			m := newTree([]Row{r}, WithSize(tt.width+1, 1), WithFocused(false))
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
	rows := []Row{
		{ID: "a", Fold: true, Closed: true, Arrow: true, Lines: []string{"closed"}},
		{ID: "b", Fold: true, Arrow: true, Lines: []string{"open"}},
		{ID: "c", Depth: 1, Lines: []string{"child"}},
	}
	got := strings.Split(plain(newTree(rows, WithSize(14, 3), WithFocused(false)).View()), "\n")
	want := []string{"▌▸ closed", " ▾ open", " ╰─ child"}
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
	rows[0].Marker, rows[0].Right, rows[0].Arrow = "⋯ 9", "right", true
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
	rows := []Row{
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
	rows := make([]Row, 1, 13)
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

// Text cut by the width keeps its links closed, and never splits a wide
// character, so the next line starts clean.
func TestViewCutsLinksAndWideText(t *testing.T) {
	link := ansi.SetHyperlink("https://example.test/x") + "a link that is long" + ansi.ResetHyperlink()
	rows := []Row{
		{ID: "l", Lines: []string{"see " + link + " now"}},
		{ID: "w", Lines: []string{"一二三四五六七八九十一二三四五六七八九十"}, Right: "界"},
		{ID: "e", Lines: []string{"日本語 é 🙂🙂🙂 end"}, Marker: "⋯", Fold: true, Closed: true},
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
	rows := []Row{{ID: "t", Lines: []string{"a\tb\tc", "\tbody", "x\nsplit"}}}
	m := newTree(rows, WithSize(14, 5), WithFocused(false))
	got := strings.Split(plain(m.View()), "\n")
	want := []string{"▌a   b   c", "▌    body", "▌x", "▌split", ""}
	for i := range want {
		if strings.TrimRight(got[i], " ") != strings.TrimRight(want[i], " ") {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

package conv

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestSnippetGolden(t *testing.T) {
	t.Parallel()
	// Each hunk ends at the line of its thread, as GitHub's do.
	const (
		upTo45  = "@@ -38,7 +38,8 @@ func (m Model) View() string {\n \tp := 0\n \tq := 1\n \tx := 1\n \ty := 2\n-\tz := 3\n+\tz := m.count()\n+\tw := z * 2\n \treturn x + y\n }"
		upToOld = "@@ -38,5 +38,4 @@ func (m Model) View() string {\n \tp := 0\n \tq := 1\n \tx := 1\n \ty := 2\n-\tz := 3"
		upTo44  = "@@ -38,6 +38,7 @@ func (m Model) View() string {\n \tp := 0\n \tq := 1\n \tx := 1\n \ty := 2\n-\tz := 3\n+\tz := m.count()\n+\tw := z * 2\n \treturn x + y"
	)
	mk := func(id, path, hunk string, line, start int, side core.Side) core.Thread {
		return core.Thread{
			ID: id, ReviewID: "R", Path: path, Line: line, StartLine: start, Side: side, TotalComments: 1,
			Comments: []core.ThreadComment{{
				ID: id + "-0", Author: core.User{Login: "ann"}, At: at, Body: "Please rename this.\n\nIt reads badly.", DiffHunk: hunk,
			}},
		}
	}
	// Code was added above since, so the thread is on lines 46 to 48 now.
	right := mk("T1", "internal/tui/model.go", upTo45, 48, 46, core.SideRight)
	right.OriginalLine, right.OriginalStartLine = 45, 43
	left := mk("T2", "internal/tui/model.go", upToOld, 42, 0, core.SideLeft)
	outdated := mk("T3", "internal/tui/model.go", upTo44, 0, 0, core.SideRight)
	outdated.Outdated, outdated.OriginalLine, outdated.OriginalStartLine = true, 44, 43
	file := mk("T4", "docs/a/very/long/directory/name/that/goes/on/and/on/for/a/while/README.md", upTo45, 0, 0, core.SideRight)
	file.File = true
	d := Data{Items: []core.TimelineItem{review("R", core.ReviewStateCommented)}, Threads: []core.Thread{right, left, outdated, file}}
	for _, set := range []string{config.IconsNerd, config.IconsASCII} {
		for _, w := range []int{80, 120} {
			t.Run(fmt.Sprintf("%s-%d", set, w), func(t *testing.T) {
				t.Parallel()
				golden.RequireEqual(t, text(Build(d, opts(set, w)), w, true))
			})
		}
	}
}

// TestSnippetMarks checks which rows of a hunk carry the accent, with hunks
// that end at the line of the thread, and a thread whose code has moved since.
func TestSnippetMarks(t *testing.T) {
	t.Parallel()
	const (
		toRight = "@@ -10,5 +10,5 @@ func f() {\n a\n b\n-c\n-d\n+e\n+f\n g"
		toLeft  = "@@ -10,4 +10,2 @@ func f() {\n a\n b\n-c\n-d"
		toMixed = "@@ -10,4 +10,4 @@ func f() {\n a\n b\n-c\n-d\n+e\n+f"
	)
	for _, tc := range []struct {
		name, hunk string
		side       core.Side
		startSide  core.Side
		line, from int
		want       []string
	}{
		{"right line", toRight, core.SideRight, "", 14, 0, []string{" g"}},
		{"right range", toRight, core.SideRight, "", 14, 12, []string{"+e", "+f", " g"}},
		{"left line", toLeft, core.SideLeft, "", 13, 0, []string{"-d"}},
		{"left range", toLeft, core.SideLeft, "", 13, 12, []string{"-c", "-d"}},
		{"start above the tail", toRight, core.SideRight, "", 14, 10, []string{"-d", "+e", "+f", " g"}},
		{"across sides", toMixed, core.SideRight, core.SideLeft, 13, 13, []string{"-d", "+e", "+f"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The thread is 5 lines further down now: only the original
			// numbers match the hunk.
			th := core.Thread{
				ID: "T", ReviewID: "R", Path: "f.go", Side: tc.side, StartSide: tc.startSide, TotalComments: 1,
				Line: tc.line + 5, OriginalLine: tc.line, OriginalStartLine: tc.from,
				Comments: []core.ThreadComment{{ID: "T-0", Author: core.User{Login: "ann"}, At: at, Body: "Rename.", DiffHunk: tc.hunk}},
			}
			if tc.from > 0 {
				th.StartLine = tc.from + 5
			}
			d := Data{Items: []core.TimelineItem{review("R", core.ReviewStateCommented)}, Threads: []core.Thread{th}}
			var got []string
			for l := range strings.SplitSeq(text(Build(d, opts(config.IconsASCII, 100)), 100, true), "\n") {
				if m := marked.FindStringSubmatch(l); m != nil {
					got = append(got, m[1])
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("marked rows %q, want %q", got, tc.want)
			}
		})
	}
}

// marked matches a snippet row that carries the accent, in the ASCII icons, and
// captures its marker and text.
var marked = regexp.MustCompile(`\| +\d* +\d* >(.*?)\s*$`)

func TestHunkTail(t *testing.T) {
	t.Parallel()
	ten := "@@ -10,6 +20,8 @@ func f() {\n a\n b\n c\n-d\n+e\n+f\n g\n h\n i\n j"
	for _, tc := range []struct {
		name, hunk string
		want       []diffLine
	}{
		{"last four", ten, []diffLine{{' ', 14, 25, "g"}, {' ', 15, 26, "h"}, {' ', 16, 27, "i"}, {' ', 17, 28, "j"}}},
		{"two lines", "@@ -1,2 +1,2 @@\n a\n b", []diffLine{{' ', 1, 1, "a"}, {' ', 2, 2, "b"}}},
		{"mixed", "@@ -5 +5,2 @@\n-x\n+y\n+z\n\\ No newline at end of file",
			[]diffLine{{'-', 5, 0, "x"}, {'+', 0, 5, "y"}, {'+', 0, 6, "z"}}},
		{"empty", "", nil},
		{"no header", " a\n b", nil},
	} {
		if got := hunkTail(tc.hunk, 4); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

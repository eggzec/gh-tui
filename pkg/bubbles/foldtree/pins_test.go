package foldtree

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// pinBG is the background of the pinned lines, #1b1f27.
const pinBG = "\x1b[48;2;27;31;39m"

// overshoot is a deep thread followed by top-level rows:
//
//	X
//	╰─ Y
//	   ╰─ Z
//	w
//	v
//	u
//	t
func overshoot() []Row {
	return []Row{
		open(fold("X", 0)), open(fold("Y", 1)), leaf("Z", 2),
		leaf("w", 0), leaf("v", 0), leaf("u", 0), leaf("t", 0),
	}
}

// tailed is deep(leaves) followed by top-level rows.
func tailed(leaves, tops int) []Row {
	rows := deep(leaves)
	for i := range tops {
		rows = append(rows, leaf(fmt.Sprintf("tail %02d", i), 0))
	}
	return rows
}

// lines returns the lines of the view without styles or trailing spaces.
func lines(m Model) []string {
	out := strings.Split(plain(m.View()), "\n")
	for i, l := range out {
		out[i] = strings.TrimRight(l, " ")
	}
	return out
}

func TestScrollPins(t *testing.T) {
	// A want of "~" is a blank line; any other is the text the line holds.
	tests := []struct {
		name   string
		rows   []Row
		height int
		keys   []string
		resize int
		want   []string
	}{
		{"end at height 1", deep(12), 1, []string{"G"}, 0,
			[]string{"leaf 11"}},
		{"end at height 2 pins the nearest ancestor", deep(12), 2, []string{"G"}, 0,
			[]string{"sub", "leaf 11"}},
		{"end at height 3", deep(12), 3, []string{"G"}, 0,
			[]string{"sec", "sub", "leaf 11"}},
		{"end at height 4", deep(12), 4, []string{"G"}, 0,
			[]string{"sec", "sub", "leaf 10", "leaf 11"}},
		{"up after the end keeps the cursor under the pins", deep(12), 4, []string{"G", "k", "k"}, 0,
			[]string{"sec", "sub", "leaf 09", "leaf 10"}},
		{"cursor on an ancestor that was pinned", deep(12), 4, append([]string{"G"}, strings.Split(strings.Repeat("k,", 13), ",")[:13]...), 0,
			[]string{"root", "sec", "sub", "leaf 00"}},
		{"cursor on the next ancestor up", deep(12), 4, append([]string{"G"}, strings.Split(strings.Repeat("k,", 14), ",")[:14]...), 0,
			[]string{"root", "sec", "sub", "leaf 00"}},
		{"cursor back at the top", deep(12), 4, append([]string{"G"}, strings.Split(strings.Repeat("k,", 15), ",")[:15]...), 0,
			[]string{"root", "sec", "sub", "leaf 00"}},
		{"shrinking to height 3", deep(12), 7, []string{"G"}, 3,
			[]string{"sec", "sub", "leaf 11"}},
		{"shrinking to height 2", deep(12), 7, []string{"G"}, 2,
			[]string{"sub", "leaf 11"}},
		{"shrinking to height 1", deep(12), 7, []string{"G"}, 1,
			[]string{"leaf 11"}},
		{"growing from height 3", deep(12), 3, []string{"G"}, 7,
			[]string{"sec", "sub", "leaf 07", "leaf 08", "leaf 09", "leaf 10", "leaf 11"}},
		{"top-level rows at the end leave no extra blank at height 5", overshoot(), 5, []string{"G"}, 0,
			[]string{"w", "v", "u", "t", "~"}},
		{"top-level rows at the end at height 4", overshoot(), 4, []string{"G"}, 0,
			[]string{"w", "v", "u", "t"}},
		{"top-level rows at the end at height 3", overshoot(), 3, []string{"G"}, 0,
			[]string{"v", "u", "t"}},
		{"blanks that the pins force", overshoot(), 6, []string{"G"}, 0,
			[]string{"w", "v", "u", "t", "~", "~"}},
		{"the thread is still pinned when the window reaches into it", overshoot(), 5, []string{"G", "k", "k"}, 0,
			[]string{"X", "Y", "Z", "w", "v"}},
		{"a resize that grows the window back", overshoot(), 5, []string{"G"}, 8,
			[]string{"X", "Y", "Z", "w", "v", "u", "t", "~"}},
		{"a resize that shrinks the window", overshoot(), 8, []string{"G"}, 5,
			[]string{"w", "v", "u", "t", "~"}},
		{"a page down out of the pins moves the least", tailed(8, 30), 8,
			append(slices.Repeat([]string{"j"}, 12), "ctrl+f"), 0,
			[]string{"tail 00", "tail 01", "tail 02", "tail 03", "tail 04", "tail 05", "tail 06", "tail 07"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTree(tt.rows, WithSize(24, tt.height))
			m, _ = keys(t, m, tt.keys...)
			h := tt.height
			if tt.resize > 0 {
				m.SetSize(24, tt.resize)
				h = tt.resize
			}
			assertFits(t, m.View(), 24, h)
			got := lines(m)
			for i, w := range tt.want {
				if w == "~" && got[i] != "" || w != "~" && !strings.Contains(got[i], w) {
					t.Fatalf("line %d = %q, want %q:\n%s", i, got[i], w, strings.Join(got, "\n"))
				}
			}
			assertShowsOnce(t, m)
		})
	}
}

// mixed is a tree with a row of three lines at depth 2, and a Skip row that
// is an ancestor of the rows under it:
//
//	root
//	╰─ group (Skip)
//	   ├─ sub
//	   ├─ tall, three lines
//	   │  ╰─ leaf 00 .. leaf 03
//	   ╰─ sub 2
//	      ╰─ leaf 04 .. leaf 06
//	tail 00 .. tail 02
func mixed() []Row {
	group := open(fold("group", 1))
	group.Skip = true
	tall := open(fold("tall", 2))
	tall.Lines = []string{"tall", "  second line", "  third line"}
	rows := []Row{open(fold("root", 0)), group, open(fold("sub", 2)), tall}
	for i := range 4 {
		rows = append(rows, leaf(fmt.Sprintf("leaf %02d", i), 3))
	}
	rows = append(rows, open(fold("sub 2", 2)))
	for i := 4; i < 7; i++ {
		rows = append(rows, leaf(fmt.Sprintf("leaf %02d", i), 3))
	}
	for i := range 3 {
		rows = append(rows, leaf(fmt.Sprintf("tail %02d", i), 0))
	}
	return rows
}

// Whatever tree, height and key, the view shows the cursor row once, never
// shows a row twice, reaches the last row, and leaves a line blank only when
// the window cannot go up without hiding the cursor row. Folding, unfolding
// and resizing between the keys changes none of that.
func TestScrollPinsKeepTheRules(t *testing.T) {
	moves := []string{"G", "k", "k", "k", "ctrl+b", "ctrl+u", "j", "j", "ctrl+d", "ctrl+f", "g", "G", "ctrl+b"}
	// A resize is a step too, and is written as "resize N".
	steps := append(slices.Clone(moves), "h", "l", "*", "j", "k", "ctrl+f", "ctrl+d", "ctrl+u", "ctrl+b", "G", "g")
	for n, rows := range [][]Row{tailed(8, 0), tailed(8, 3), overshoot(), tailed(1, 6), mixed()} {
		for h := 1; h <= 10; h++ {
			rng := rand.New(rand.NewPCG(uint64(n), uint64(h)))
			m := newTree(rows, WithSize(24, h))
			run := slices.Clone(moves)
			for range 120 {
				if rng.IntN(4) == 0 {
					run = append(run, fmt.Sprintf("resize %d", 1+rng.IntN(10)))
					continue
				}
				run = append(run, steps[rng.IntN(len(steps))])
			}
			trail := fmt.Sprintf("tree %d, height %d:", n, h)
			for _, k := range run {
				// from is the window before a move; a fold, a resize or the
				// first step has none.
				from := -1
				if size, ok := strings.CutPrefix(k, "resize "); ok {
					m.SetSize(24, atoi(t, size))
				} else {
					if !slices.Contains([]string{"h", "l", "*"}, k) {
						from = m.off
					}
					m, _ = keys(t, m, k)
				}
				trail += " " + k
				checkPins(t, m, k, trail, from)
			}
		}
	}
}

func atoi(tb testing.TB, s string) int {
	tb.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		tb.Fatal(err)
	}
	return n
}

// checkPins checks the rules of the scroll on the view of m after step. A
// from of 0 or more is the line the window started at before a move.
func checkPins(t *testing.T, m Model, step, trail string, from int) {
	t.Helper()
	h := m.height
	name := fmt.Sprintf("%d rows, height %d, after %s\n%s", m.Len(), h, step, trail)
	assertFits(t, m.View(), 24, h)
	assertShowsOnce(t, m)
	got := lines(m)
	seen := map[string]bool{}
	for _, l := range got {
		if l != "" && seen[l] {
			t.Fatalf("%s: line %q twice:\n%s", name, l, strings.Join(got, "\n"))
		}
		seen[l] = true
	}
	if len(m.pins) > min(maxPins, h-1) {
		t.Fatalf("%s: %d pins", name, len(m.pins))
	}
	blanks := 0
	for _, l := range got {
		if l == "" {
			blanks++
		}
	}
	if blanks > 0 && m.off > 0 {
		e := m.starts[m.cur+1]
		if e <= m.off-1+m.height-len(m.pinsAt(m.off-1)) {
			t.Fatalf("%s: %d blank lines but the window could go up:\n%s", name, blanks, strings.Join(got, "\n"))
		}
	}
	// A move that took the window down takes it the least: a line less would
	// still show the cursor row under the pins of that place. When the row at
	// that line is pinned now, it is the same view with the row in the place
	// of its pin.
	if from >= 0 && m.off > from {
		s, e := m.starts[m.cur], m.starts[m.cur+1]
		up := m.off - 1
		top := sort.Search(len(m.vis), func(i int) bool { return m.starts[i+1] > up })
		if e <= up+m.height-len(m.pinsAt(up)) && s >= up+len(m.pinsAt(up)) && !slices.Contains(m.pins, top) {
			t.Fatalf("%s: the window went down to line %d from %d but line %d shows the cursor row:\n%s", name, m.off, from, up, strings.Join(got, "\n"))
		}
	}
	if step == "G" || step == "ctrl+f" && m.cur == len(m.vis)-1 {
		last := m.row(len(m.vis) - 1).ID
		if !strings.HasSuffix(got[len(got)-1-blanks], last) {
			t.Fatalf("%s: the last row %q is not the last line:\n%s", name, last, strings.Join(got, "\n"))
		}
	}
}

// The pinned lines are on a background of their own, the rows are not, and
// the cursor row keeps the background of the cursor.
func TestViewPinnedBackground(t *testing.T) {
	m := newTree(deep(12), WithSize(24, 6))
	m, _ = keys(t, m, "G", "k")
	v := m.View()
	assertFits(t, v, 24, 6)
	golden.RequireEqual(t, v)
	for i, l := range strings.Split(v, "\n") {
		switch has := strings.Contains(l, pinBG); {
		case i < len(m.pins) && !has:
			t.Errorf("pinned line %d has no pin background: %q", i, l)
		case i >= len(m.pins) && has:
			t.Errorf("line %d is not pinned but has the pin background: %q", i, l)
		}
		if strings.Contains(l, rowBG) && strings.Contains(l, pinBG) {
			t.Errorf("line %d has both backgrounds: %q", i, l)
		}
	}
}

// A pinned row that is the cursor row is drawn as the cursor row.
func TestViewPinnedCursorRow(t *testing.T) {
	m := newTree(deep(12), WithSize(24, 6))
	m, _ = keys(t, m, "G")
	m.cur = m.pins[0]
	line := m.pinned(m.cur)
	if strings.Contains(line, pinBG) || !strings.Contains(line, rowBG) || !strings.Contains(plain(line), "▌") {
		t.Errorf("pinned cursor row = %q, want the cursor row", line)
	}
}

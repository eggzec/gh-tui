package pager

import (
	"slices"
	"testing"
)

// foldMask shows every row of n but rows 3 to 9, which a fold hides under
// row 2.
func foldMask(n int) []int32 {
	var vis []int32
	for i := range n {
		if i < 3 || i > 9 {
			vis = append(vis, int32(i))
		}
	}
	return vis
}

func foldAnchor(int) int { return 2 }

func maskPager(tb testing.TB, opts ...Option) (Model, *liveRows) {
	tb.Helper()
	l := liveOf(30, "row")
	m := fresh(tb, append([]Option{WithSize(40, 8), WithRows(l)}, opts...)...)
	m.Focus()
	return m, l
}

func TestMaskHidesRowsAndNilRestoresThem(t *testing.T) {
	m, _ := maskPager(t)
	m.SetVisible(foldMask(30), foldAnchor)
	if m.Shown() != 23 {
		t.Fatalf("%d shown, want 23", m.Shown())
	}
	m.SetVisible(nil, nil)
	if m.Shown() != 30 || m.vis != nil || m.mask != nil {
		t.Errorf("%d shown after SetVisible(nil), vis %v", m.Shown(), m.vis)
	}
}

func TestMaskMovesTheCursorToTheAnchor(t *testing.T) {
	m, _ := maskPager(t, WithCursor(true))
	m.SetCursor(5)
	m.SetVisible(foldMask(30), foldAnchor)
	if m.Cursor() != 2 {
		t.Errorf("cursor on %d, want the anchor 2", m.Cursor())
	}
}

func TestMaskPlacesTheWindowOnTheAnchor(t *testing.T) {
	for _, wrap := range []bool{true, false} {
		m, _ := maskPager(t)
		m.wrap = wrap
		m.top = 5
		m.SetVisible(foldMask(30), foldAnchor)
		if m.topLine() != 2 {
			t.Errorf("wrap %v: window at row %d, want the anchor 2", wrap, m.topLine())
		}
		// Position 3 is row 10 among the rows shown.
		m.top = 3
		m.SetVisible(foldMask(30), foldAnchor)
		if m.topLine() != 10 {
			t.Errorf("wrap %v: a shown top row moved to %d", wrap, m.topLine())
		}
	}
}

func TestMaskComposesWithSqueeze(t *testing.T) {
	l := &liveRows{lines: []string{"a", "", "", "b", "", "", "c"}}
	m := fresh(t, WithSize(40, 8), WithRows(l))
	_ = m.project(projection{squeeze: true})
	if !slices.Equal(m.vis, []int32{0, 1, 3, 4, 6}) {
		t.Fatalf("squeeze shows %v", m.vis)
	}
	m.SetVisible([]int32{0, 1, 3, 6}, nil)
	if !slices.Equal(m.vis, []int32{0, 1, 3, 6}) {
		t.Errorf("mask and squeeze show %v", m.vis)
	}
	m.SetVisible(nil, nil)
	if !slices.Equal(m.vis, []int32{0, 1, 3, 4, 6}) {
		t.Errorf("without the mask %v", m.vis)
	}
}

func TestMaskKeepsAPendingFilter(t *testing.T) {
	m, _ := maskPager(t)
	m.size = syncLimit
	cmd := m.filterFor("row 1")
	if cmd == nil || !m.Projecting() {
		t.Fatal("setup: the filter is not pending")
	}
	pgen := m.pgen
	m.SetVisible(foldMask(30), foldAnchor)
	if !m.Projecting() || m.pgen != pgen {
		t.Errorf("the fold stopped the filter: projecting %v, pgen %d to %d", m.Projecting(), pgen, m.pgen)
	}
	m = settle(m, cmd)
	// row 1 and row 10 to row 19 are picked, and the fold hides none of them.
	if m.Shown() != 11 || m.mask == nil {
		t.Errorf("%d shown, want 11 with the mask kept", m.Shown())
	}
	m.SetVisible([]int32{1, 10, 11}, nil)
	if !slices.Equal(m.vis, []int32{1, 10, 11}) {
		t.Errorf("the new mask shows %v", m.vis)
	}
}

func TestMaskSurvivesAStaleFilterResult(t *testing.T) {
	m, _ := maskPager(t)
	m.size = syncLimit
	stale := m.filterFor("row 1")
	cur := m.filterFor("row 2")
	m.SetVisible([]int32{0, 20, 21, 29}, nil)
	m, _ = m.Update(stale())
	if !m.Projecting() || m.Shown() != 4 {
		t.Fatalf("a stale result was taken: projecting %v, %d shown", m.Projecting(), m.Shown())
	}
	m = settle(m, cur)
	// row 2 and row 20 to row 29, among the rows the mask shows.
	if !slices.Equal(m.vis, []int32{20, 21, 29}) {
		t.Errorf("the filter result overwrote the mask: %v", m.vis)
	}
}

func TestMaskSearchOpensAClosedFold(t *testing.T) {
	var asked []int
	m, _ := maskPager(t, WithReveal(func(row int) ([]int32, bool) {
		asked = append(asked, row)
		return nil, true
	}))
	m.SetVisible(foldMask(30), foldAnchor)
	_ = m.SetSearch("row 5")
	if !slices.Equal(asked, []int{5}) {
		t.Fatalf("hook asked for %v", asked)
	}
	if m.Shown() != 30 || m.search.curLine != 5 || m.Matches() != 1 {
		t.Errorf("%d shown, match on %d, %d matches", m.Shown(), m.search.curLine, m.Matches())
	}
}

func TestMaskRevealRefusedKeepsTheMatchHidden(t *testing.T) {
	m, _ := maskPager(t, WithReveal(func(int) ([]int32, bool) { return nil, false }))
	m.SetVisible(foldMask(30), foldAnchor)
	_ = m.SetSearch("row 5")
	if m.Shown() != 23 || m.topLine() == 5 || m.flash != noteNotFound {
		t.Errorf("%d shown, top %d, note %q", m.Shown(), m.topLine(), m.flash)
	}
}

func TestMaskSearchWithoutAHookIsOverTheShownRows(t *testing.T) {
	m, _ := maskPager(t)
	m.SetVisible(foldMask(30), foldAnchor)
	_ = m.SetSearch("row 5")
	if m.Matches() != 0 || m.flash != noteNotFound {
		t.Errorf("%d matches, note %q", m.Matches(), m.flash)
	}
	_ = m.SetSearch("row 1")
	// row 1 and row 10 to row 19.
	if m.Matches() != 11 {
		t.Errorf("%d matches in the rows shown, want 11", m.Matches())
	}
}

func TestMaskRowsAppendedStayHiddenUntilSetVisible(t *testing.T) {
	m, l := maskPager(t)
	m.SetVisible(foldMask(30), foldAnchor)
	l.add("row", 30, 33)
	m.Append(30)
	if m.Shown() != 23 {
		t.Errorf("%d shown after Append, want 23", m.Shown())
	}
	m.SetVisible(foldMask(33), foldAnchor)
	if m.Shown() != 26 {
		t.Errorf("%d shown, want 26", m.Shown())
	}
}

func TestMaskFollowsTheDocument(t *testing.T) {
	m := docsPager(t, Doc{Name: "a.txt", Text: numbered(30)}, Doc{Name: "b.txt", Text: numbered(30)})
	all := m.Shown()
	m.SetVisible(foldMask(all), foldAnchor)
	m = step(t, m, 1)
	if m.Shown() != all || m.mask != nil {
		t.Errorf("the other document shows %d of %d", m.Shown(), all)
	}
	m = step(t, m, -1)
	if m.Shown() != all-7 || m.mask == nil {
		t.Errorf("the first document shows %d, want %d", m.Shown(), all-7)
	}
}

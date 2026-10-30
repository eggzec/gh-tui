package pager

import "testing"

func TestSetCursorWithWrapKeepsTheCursorInView(t *testing.T) {
	for _, row := range []int{20, 29, 0} {
		m := fresh(t, WithSize(40, 8), WithRows(liveOf(30, "row")), WithCursor(true))
		m.Focus()
		m.wrap = true
		m.SetCursor(row)
		if m.Cursor() != row || !m.covers(m.curPos()) {
			t.Errorf("SetCursor(%d): cursor %d, in view %v", row, m.Cursor(), m.covers(m.curPos()))
		}
	}
}

func TestSearchJumpWithWrapLandsTheCursor(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithRows(liveOf(30, "row")), WithCursor(true))
	m.Focus()
	m.wrap = true
	_ = m.SetSearch("row 25")
	if m.search.curLine != 25 || m.Cursor() != 25 || !m.covers(m.curPos()) {
		t.Errorf("match on %d, cursor %d, in view %v", m.search.curLine, m.Cursor(), m.covers(m.curPos()))
	}
}

func TestSetPositionBeforeContentSnapsToTheStart(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithCursor(true))
	m.SetPosition(Position{Cursor: 5, Top: 7, Row: 2, Left: 3})
	if m.top != 0 || m.row != 0 {
		t.Errorf("window at %d row %d, want the start", m.top, m.row)
	}
}

func TestMaskRevealThatLeavesTheRowHiddenFlashesNotFound(t *testing.T) {
	m, _ := maskPager(t, WithReveal(func(int) ([]int32, bool) { return foldMask(30), true }))
	m.SetVisible(foldMask(30), foldAnchor)
	_ = m.SetSearch("row 5")
	if m.flash != noteNotFound || m.Shown() != 23 || m.search.cur != -1 {
		t.Errorf("note %q, %d shown, current %d", m.flash, m.Shown(), m.search.cur)
	}
}

func TestMaskRevealKeepsTheCounterWhenRefused(t *testing.T) {
	m, _ := maskPager(t, WithReveal(func(row int) ([]int32, bool) { return nil, row != 5 }))
	m.SetVisible(foldMask(30), foldAnchor)
	_ = m.SetSearch("row")
	if m.search.cur != 0 {
		t.Fatalf("setup: current match %d", m.search.cur)
	}
	// Every row matches, so match 5 is on row 5, which the fold hides.
	m.jump(5)
	if m.search.cur != 0 || m.search.curLine != 0 || m.flash != noteNotFound {
		t.Errorf("counter moved to %d (line %d), note %q", m.search.cur, m.search.curLine, m.flash)
	}
}

func TestMaskRevealWithTheCursor(t *testing.T) {
	m, _ := maskPager(t, WithCursor(true), WithReveal(func(int) ([]int32, bool) { return nil, true }))
	m.SetVisible(foldMask(30), foldAnchor)
	_ = m.SetSearch("row 5")
	if m.Cursor() != 5 || m.Shown() != 30 {
		t.Errorf("cursor on %d, %d shown", m.Cursor(), m.Shown())
	}
}

func TestMaskRevealWithoutAMaskLeavesNoAnchor(t *testing.T) {
	m, _ := maskPager(t, WithReveal(func(int) ([]int32, bool) { return nil, true }))
	m.SetVisible(foldMask(30), foldAnchor)
	_ = m.SetSearch("row 5")
	if m.mask != nil || m.maskAnchor != nil {
		t.Errorf("mask %v and anchor set after the mask was lifted", m.mask)
	}
}

func TestMaskWithFilterAndAppend(t *testing.T) {
	m, l := maskPager(t)
	m.SetVisible(foldMask(30), foldAnchor)
	m = settle(m, m.filterFor("row 3"))
	// row 3 is hidden by the fold.
	if m.Shown() != 0 {
		t.Fatalf("%d shown, want 0", m.Shown())
	}
	l.add("row", 30, 33)
	m.Append(30)
	if m.Shown() != 0 {
		t.Errorf("%d shown after Append, want the new rows hidden by the mask", m.Shown())
	}
	m.SetVisible(foldMask(33), foldAnchor)
	if m.Shown() != 3 {
		t.Errorf("%d shown, want rows 30 to 32", m.Shown())
	}
}

func TestMaskRevealWithTwoDocs(t *testing.T) {
	var asked []int
	m := fresh(t, WithSize(40, 8), WithReveal(func(row int) ([]int32, bool) {
		asked = append(asked, row)
		return nil, true
	}))
	m.Focus()
	m = settle(m, m.SetDocs([]Doc{{Name: "a.txt", Text: numbered(30)}, {Name: "b.txt", Text: numbered(30)}}))
	m.SetVisible(foldMask(m.Shown()), foldAnchor)
	m = step(t, m, 1)
	if m.mask != nil {
		t.Fatal("the mask followed to the other document")
	}
	_ = m.SetSearch("5")
	if len(asked) != 0 {
		t.Errorf("the hook was asked for %v in a document with no mask", asked)
	}
}

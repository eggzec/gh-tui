package pager

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// liveRows is a host's rows that grow, as a log does.
type liveRows struct{ lines []string }

func (l *liveRows) Len() int                   { return len(l.lines) }
func (l *liveRows) Text(i int) string          { return l.lines[i] }
func (l *liveRows) Marks(int) []termtext.Style { return nil }

func (l *liveRows) add(word string, from, to int) {
	for i := from; i < to; i++ {
		l.lines = append(l.lines, word+" "+strconv.Itoa(i))
	}
}

func liveOf(n int, word string) *liveRows {
	l := &liveRows{}
	l.add(word, 0, n)
	return l
}

// same fails unless the pager holds what one that was given all the rows at
// once does.
func same(t *testing.T, m Model, l *liveRows) {
	t.Helper()
	ref := fresh(t, WithSize(m.width, m.height), WithRows(l))
	if m.size != ref.size || m.seen != l.Len() {
		t.Errorf("size %d, seen %d; want %d, %d", m.size, m.seen, ref.size, l.Len())
	}
	if m.search.re != nil {
		s := m.search
		lines, ends, _ := findFrom(context.Background(), s.re, s.invert, l, nil, 0, nil, nil)
		if !slices.Equal(s.lines, lines) || !slices.Equal(s.ends, ends) {
			t.Errorf("search found %d lines, a search of all the rows %d", len(s.lines), len(lines))
		}
	}
}

func TestAppendKeepsTheSearch(t *testing.T) {
	l := liveOf(50, "entry")
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	m = typeText(t, m, "/")
	m = typeText(t, m, "entry 3")
	m, _ = keys(t, m, "enter")
	m, _ = keys(t, m, "n")
	cur, top := m.search.cur, m.top
	if m.Matches() != 11 || cur != 1 {
		t.Fatalf("setup: %d matches, at %d", m.Matches(), cur)
	}
	l.add("entry", 50, 40+40)
	m.Append(50)
	if got := m.Matches(); got != 11+ /* 50..79 has none with "entry 3" */ 0 {
		t.Errorf("%d matches, want 11", got)
	}
	l.add("entry", 300, 303)
	m.Append(80)
	if m.Matches() != 14 || m.search.cur != cur || m.top != top || m.Query() != "entry 3" {
		t.Errorf("%d matches, at %d, top %d, query %q; want 14 at %d, top %d", m.Matches(), m.search.cur, m.top, m.Query(), cur, top)
	}
	// Back from the first match wraps to the last, which is new.
	m, _ = keys(t, m, "N", "N")
	if m.search.cur != 13 {
		t.Errorf("the search stepped to %d, want the new match 13", m.search.cur)
	}
	same(t, m, l)
}

func TestAppendKeepsTheFilter(t *testing.T) {
	l := liveOf(30, "row")
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	_ = m.filterFor("row 1")
	// row 1, row 10 to row 19.
	if m.Shown() != 11 {
		t.Fatalf("setup: %d shown", m.Shown())
	}
	l.add("row", 30, 32)
	l.add("other", 0, 3)
	m.Append(30)
	// row 30 and row 31 do not match "row 1".
	if m.Shown() != 11 || m.Filter() != "row 1" {
		t.Errorf("%d shown, filter %q", m.Shown(), m.Filter())
	}
	l.add("row", 100, 103)
	m.Append(35)
	if m.Shown() != 14 || m.kept != 14 {
		t.Errorf("%d shown, %d kept, want 14", m.Shown(), m.kept)
	}
	if v := m.View(); strings.Contains(v, "other") {
		t.Errorf("a row the filter hides is shown:\n%s", v)
	}
}

func TestAppendSqueezesNewRows(t *testing.T) {
	l := &liveRows{lines: []string{"a", "", ""}}
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	m, _ = keys(t, m, "-", "s")
	if m.Shown() != 2 {
		t.Fatalf("setup: %d shown, want 2", m.Shown())
	}
	l.lines = append(l.lines, "", "b", "", "")
	m.Append(3)
	// a, one blank, b, one blank.
	if m.Shown() != 4 {
		t.Errorf("%d shown after append, want 4", m.Shown())
	}
}

func TestFollowPinsTheEnd(t *testing.T) {
	l := liveOf(50, "entry")
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	if m.Following() {
		t.Fatal("a pager follows before it is told to")
	}
	m.SetFollow(true)
	if !m.Following() || m.top != 50-7 {
		t.Fatalf("following %v, top %d, want the end", m.Following(), m.top)
	}
	l.add("entry", 50, 60)
	m.Append(50)
	if m.top != 60-7 || !strings.Contains(m.View(), "entry 59") {
		t.Errorf("top %d; the view doesn't end at the last row:\n%s", m.top, m.View())
	}
	m, _ = keys(t, m, "k")
	if m.Following() {
		t.Error("scrolling up kept following")
	}
	top := m.top
	l.add("entry", 60, 70)
	m.Append(60)
	if m.top != top {
		t.Errorf("top moved to %d from %d without following", m.top, top)
	}
	m.SetFollow(true)
	if m.top != 70-7 {
		t.Errorf("top %d after SetFollow, want the end", m.top)
	}
	m, _ = keys(t, m, "j")
	if !m.Following() {
		t.Error("a key that stays at the end stopped following")
	}
}

func TestFollowWraps(t *testing.T) {
	l := &liveRows{lines: []string{strings.Repeat("word ", 12)}}
	m := fresh(t, WithSize(20, 6), WithWrap(true), WithRows(l))
	m.SetFollow(true)
	l.lines = append(l.lines, "tail one", strings.Repeat("long ", 10))
	m.Append(1)
	last, lastRow := m.last()
	if m.top != last || m.row != lastRow {
		t.Errorf("window at %d/%d, want %d/%d", m.top, m.row, last, lastRow)
	}
}

func TestAppendHandlesAnyFrom(t *testing.T) {
	l := liveOf(10, "entry")
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	m = typeText(t, m, "/")
	m = typeText(t, m, "entry")
	m, _ = keys(t, m, "enter")
	for _, from := range []int{-3, 0, 5, 10, 11, 1000} {
		l.add("entry", 0, 2)
		m.Append(from)
		if m.seen != l.Len() || m.Matches() != l.Len() {
			t.Errorf("from %d: seen %d, %d matches, want %d", from, m.seen, m.Matches(), l.Len())
		}
		same(t, m, l)
	}
	// Rows went.
	l.lines = l.lines[:5]
	m.Append(5)
	if m.seen != 5 || m.Matches() != 5 {
		t.Errorf("after a shrink: seen %d, %d matches", m.seen, m.Matches())
	}
	// Nothing new.
	m.Append(5)
	m.Append(9)
}

func TestAppendOnEmpty(t *testing.T) {
	l := &liveRows{}
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Append(0)
	if m.Lines() != 0 {
		t.Fatalf("%d lines", m.Lines())
	}
	m.SetFollow(true)
	l.add("entry", 0, 3)
	m.Append(0)
	if v := m.View(); !strings.Contains(v, "entry 2") || m.Lines() != 3 {
		t.Errorf("the new rows aren't shown:\n%s", v)
	}
}

func TestAppendIgnoresContent(t *testing.T) {
	m := fresh(t, WithSize(40, 8))
	m = settle(m, m.SetContent("a.txt", numbered(20)))
	if cmd := m.Append(5); cmd != nil || m.Lines() != 20 {
		t.Errorf("Append changed content: %d lines", m.Lines())
	}
}

// bigRows has enough text that the filter and the search run in commands.
func bigRows(n int) *liveRows {
	l := &liveRows{}
	pad := strings.Repeat("x", 120)
	for i := range n {
		l.lines = append(l.lines, "row "+strconv.Itoa(i)+" "+pad)
	}
	return l
}

func TestAppendWhileTheFilterPicks(t *testing.T) {
	l := bigRows(3000)
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	cmd := m.filterFor("row 12")
	if cmd == nil || !m.Projecting() {
		t.Fatalf("setup: the filter did not run in a command")
	}
	msg := cmd()
	// row 12, row 120 to 129 and row 1200 to 1299.
	l.lines = append(l.lines, "row 12 appended", "nothing")
	m.Append(3000)
	m, _ = deliver(m, msg)
	if m.Projecting() || m.Shown() != 111+1 || m.kept != 112 {
		t.Errorf("projecting %v, %d shown, %d kept, want 112 with the appended row", m.Projecting(), m.Shown(), m.kept)
	}
	if v := m.View(); m.Shown() != 112 || v == "" {
		t.Errorf("view: %s", v)
	}
}

func TestStaleProjectionIsDroppedAfterAppend(t *testing.T) {
	l := bigRows(3000)
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	old := m.filterFor("row 12")()
	cmd := m.filterFor("row 99")
	l.lines = append(l.lines, "row 99 appended")
	m.Append(3000)
	m, _ = deliver(m, old)
	if !m.Projecting() || m.Filter() != "" {
		t.Errorf("a stale projection was taken: projecting %v, filter %q", m.Projecting(), m.Filter())
	}
	m, _ = deliver(m, cmd())
	// row 99, row 990 to 999 and the appended one.
	if m.Filter() != "row 99" || m.Shown() != 12 {
		t.Errorf("filter %q, %d shown, want 12", m.Filter(), m.Shown())
	}
}

func TestAppendWhileTheSearchRuns(t *testing.T) {
	l := bigRows(3000)
	m := fresh(t, WithSize(40, 8), WithRows(l))
	m.Focus()
	m = typeText(t, m, "/")
	m = typeText(t, m, "row 7")
	m, cmd := m.Update(enter)
	if !m.search.running || cmd == nil {
		t.Fatalf("setup: the search did not run in a command (running %v)", m.search.running)
	}
	msg := cmd()
	l.lines = append(l.lines, "row 7 appended")
	m.Append(3000)
	if m.search.lines != nil {
		t.Fatal("Append filled a search that is still running")
	}
	m, _ = deliver(m, msg)
	// row 7, row 70 to 79, row 700 to 799, and the appended one.
	if got := m.Matches(); got != 112 {
		t.Errorf("%d matches, want 112", got)
	}
	same(t, m, l)
}

func TestDocsKeepTheirFollow(t *testing.T) {
	m := docsPager(t, Doc{Name: "a.log", Text: numbered(5)}, Doc{Name: "b.log", Text: numbered(5)})
	a, b := liveOf(30, "a"), liveOf(30, "b")
	m.rows, m.seen = a, a.Len()
	m.SetFollow(true)
	m = step(t, m, 1)
	if m.Following() {
		t.Error("the second document follows with the first")
	}
	m.rows, m.seen = b, b.Len()
	a.add("a", 30, 40)
	m = step(t, m, -1)
	if !m.Following() || m.Lines() != 40 || m.seen != 40 || m.top != 40-7 {
		t.Errorf("first document: following %v, %d lines, seen %d, top %d", m.Following(), m.Lines(), m.seen, m.top)
	}
	m = step(t, m, 1)
	if m.Following() || m.Lines() != 30 {
		t.Errorf("second document: following %v, %d lines", m.Following(), m.Lines())
	}
}

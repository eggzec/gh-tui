package pager

import (
	"strings"
	"testing"
)

// cursored returns a focused pager of 50 numbered lines, 8 rows high, with a
// cursor.
func cursored(t *testing.T, opts ...Option) Model {
	t.Helper()
	opts = append([]Option{WithSize(40, 8), WithCursor(true)}, opts...)
	m := fresh(t, opts...)
	m.Focus()
	if cmd := m.SetContent("f.txt", numbered(50)); cmd != nil {
		m, _ = m.Update(cmd())
	}
	return m
}

func TestCursorOffChangesNothing(t *testing.T) {
	t.Parallel()
	m := fresh(t, WithSize(40, 8))
	m.Focus()
	m.SetContent("f.txt", numbered(50))
	if m.Cursoring() || m.Cursor() != 0 {
		t.Fatalf("cursoring %v, cursor %d", m.Cursoring(), m.Cursor())
	}
	m, _ = keys(t, m, "j", "j", "space", "ctrl+d")
	if m.top != 7+3+2 || m.Cursor() != 0 {
		t.Errorf("top %d, cursor %d; want the window to scroll alone", m.top, m.Cursor())
	}
	m.SetCursor(30)
	if m.Cursor() != 0 || m.top != 12 {
		t.Errorf("SetCursor did something: cursor %d, top %d", m.Cursor(), m.top)
	}
}

func TestCursorLines(t *testing.T) {
	t.Parallel()
	for _, wrap := range []bool{false, true} {
		m := cursored(t, WithWrap(wrap))
		m, _ = keys(t, m, "j", "j", "j")
		if m.Cursor() != 3 || m.top != 0 {
			t.Fatalf("wrap %v: cursor %d, top %d; want 3, 0", wrap, m.Cursor(), m.top)
		}
		for range 10 {
			m, _ = keys(t, m, "j")
		}
		if m.Cursor() != 13 || m.top != 7 {
			t.Errorf("wrap %v: cursor %d, top %d; want 13, 7", wrap, m.Cursor(), m.top)
		}
		for range 7 {
			m, _ = keys(t, m, "k")
		}
		if m.Cursor() != 6 || m.top != 6 {
			t.Errorf("wrap %v: cursor %d, top %d; want 6, 6", wrap, m.Cursor(), m.top)
		}
		m, _ = keys(t, m, "k", "k", "k", "k", "k", "k", "k", "k")
		if m.Cursor() != 0 || m.top != 0 {
			t.Errorf("wrap %v: cursor %d, top %d; want 0, 0", wrap, m.Cursor(), m.top)
		}
	}
}

func TestCursorPagesHomeEnd(t *testing.T) {
	t.Parallel()
	for _, wrap := range []bool{false, true} {
		m := cursored(t, WithWrap(wrap))
		m, _ = keys(t, m, "space")
		if m.Cursor() != 7 || m.top != 7 {
			t.Errorf("wrap %v: page: cursor %d, top %d; want 7, 7", wrap, m.Cursor(), m.top)
		}
		m, _ = keys(t, m, "ctrl+d")
		if m.Cursor() != 10 || m.top != 10 {
			t.Errorf("wrap %v: half page: cursor %d, top %d; want 10, 10", wrap, m.Cursor(), m.top)
		}
		m, _ = keys(t, m, "b")
		if m.Cursor() != 3 || m.top != 3 {
			t.Errorf("wrap %v: page up: cursor %d, top %d; want 3, 3", wrap, m.Cursor(), m.top)
		}
		m, _ = keys(t, m, "G")
		if m.Cursor() != 49 || m.top != 43 {
			t.Errorf("wrap %v: end: cursor %d, top %d; want 49, 43", wrap, m.Cursor(), m.top)
		}
		m, _ = keys(t, m, "space")
		if m.Cursor() != 49 {
			t.Errorf("wrap %v: page past the end: cursor %d", wrap, m.Cursor())
		}
		m, _ = keys(t, m, "g")
		if m.Cursor() != 0 || m.top != 0 {
			t.Errorf("wrap %v: home: cursor %d, top %d", wrap, m.Cursor(), m.top)
		}
	}
}

func TestCursorWrappedLines(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("aaaa bbbb cccc dddd eeee ffff\n", 30)
	m := fresh(t, WithSize(20, 8), WithCursor(true), WithWrap(true))
	m.Focus()
	m.SetContent("f.txt", long)
	for i := 1; i <= 29; i++ {
		m, _ = keys(t, m, "j")
		if m.Cursor() != i || !m.covers(m.curPos()) {
			t.Fatalf("after %d lines: cursor %d, top %d row %d, not in the window", i, m.Cursor(), m.top, m.row)
		}
	}
	for i := 28; i >= 0; i-- {
		m, _ = keys(t, m, "k")
		if m.Cursor() != i || !m.covers(m.curPos()) {
			t.Fatalf("up to %d: cursor %d, top %d row %d, not in the window", i, m.Cursor(), m.top, m.row)
		}
	}
}

func TestSetCursorReveals(t *testing.T) {
	t.Parallel()
	m := cursored(t)
	m.SetCursor(30)
	if m.Cursor() != 30 || !m.covers(30) {
		t.Errorf("cursor %d, top %d; want 30 in the window", m.Cursor(), m.top)
	}
	m.SetCursor(999)
	if m.Cursor() != 49 || !m.covers(49) {
		t.Errorf("cursor %d, top %d; want 49 in the window", m.Cursor(), m.top)
	}
	m.SetCursor(-5)
	if m.Cursor() != 0 || m.top != 0 {
		t.Errorf("cursor %d, top %d; want 0, 0", m.Cursor(), m.top)
	}
}

func TestSearchMovesCursor(t *testing.T) {
	t.Parallel()
	m := cursored(t)
	m = typeText(t, m, "/")
	m = typeText(t, m, "xxxxxx 2")
	m, _ = keys(t, m, "enter")
	if m.Cursor() != 20 || !m.covers(20) {
		t.Fatalf("cursor %d, want 20 in the window", m.Cursor())
	}
	m, _ = keys(t, m, "n")
	if m.Cursor() != 27 {
		t.Errorf("next: cursor %d, want 27", m.Cursor())
	}
	m, _ = keys(t, m, "N")
	if m.Cursor() != 20 {
		t.Errorf("previous: cursor %d, want 20", m.Cursor())
	}
}

func TestAppendKeepsTheCursor(t *testing.T) {
	t.Parallel()
	l := liveOf(20, "entry")
	m := fresh(t, WithSize(40, 8), WithRows(l), WithCursor(true))
	m.Focus()
	m, _ = keys(t, m, "j", "j")
	l.add("entry", 20, 30)
	m.Append(20)
	if m.Cursor() != 2 || m.top != 0 {
		t.Errorf("cursor %d, top %d; want 2, 0", m.Cursor(), m.top)
	}
	m.SetFollow(true)
	if m.Cursor() != 29 {
		t.Errorf("following: cursor %d, want 29", m.Cursor())
	}
	l.add("entry", 30, 35)
	m.Append(30)
	if m.Cursor() != 34 || !m.Following() {
		t.Errorf("following: cursor %d, follow %v; want 34, true", m.Cursor(), m.Following())
	}
	m, _ = keys(t, m, "k")
	if m.Following() || m.Cursor() != 33 {
		t.Errorf("after k: follow %v, cursor %d; want false, 33", m.Following(), m.Cursor())
	}
	l.add("entry", 35, 40)
	m.Append(35)
	if m.Cursor() != 33 {
		t.Errorf("not following: cursor %d, want 33", m.Cursor())
	}
}

func TestFilterMovesCursor(t *testing.T) {
	t.Parallel()
	// Lines 6, 13, 20, ... have six x.
	for _, c := range []struct{ from, want int }{{8, 13}, {5, 6}, {49, 48}} {
		m := cursored(t)
		m.SetCursor(c.from)
		m = typeText(t, m, "&")
		m = typeText(t, m, "xxxxxx")
		m, _ = keys(t, m, "enter")
		if m.Cursor() != c.want || !m.covers(m.curPos()) {
			t.Errorf("from %d: cursor %d, want %d in the window", c.from, m.Cursor(), c.want)
		}
	}
}

func TestCursorPerDocument(t *testing.T) {
	t.Parallel()
	m := fresh(t, WithSize(40, 8), WithCursor(true))
	m.Focus()
	m.SetDocs([]Doc{{Name: "a.txt", Text: numbered(50)}, {Name: "b.txt", Text: numbered(40)}})
	m.SetCursor(30)
	saved := m.Position()
	if saved.Cursor != 30 {
		t.Fatalf("position %+v", saved)
	}
	m.Step(1)
	if m.Cursor() != 0 {
		t.Errorf("second document: cursor %d, want 0", m.Cursor())
	}
	m.SetCursor(12)
	m.Step(-1)
	if m.Position() != saved {
		t.Errorf("back: position %+v, want %+v", m.Position(), saved)
	}
	m.SetCursor(2)
	m.SetPosition(saved)
	if m.Position() != saved {
		t.Errorf("restored: position %+v, want %+v", m.Position(), saved)
	}
	m.SetPosition(Position{Cursor: 900, Top: 900, Row: 5, Left: -3})
	if m.Cursor() != 49 || m.top > 49 || m.row != 0 || m.left != 0 {
		t.Errorf("clamped: cursor %d, top %d, row %d, left %d", m.Cursor(), m.top, m.row, m.left)
	}
	m.Step(1)
	if m.Cursor() != 12 {
		t.Errorf("second document again: cursor %d, want 12", m.Cursor())
	}
}

// markGutter is a host gutter that marks the cursor's row.
type markGutter struct{}

func (markGutter) Width(int) int { return 2 }

func (markGutter) Write(b *strings.Builder, _ int, _ bool) { b.WriteString("  ") }

func (markGutter) WriteCursor(b *strings.Builder, _ int, first, cursor bool) {
	if first && cursor {
		b.WriteString("> ")
		return
	}
	b.WriteString("  ")
}

func TestHostGutterDrawsCursor(t *testing.T) {
	t.Parallel()
	m := cursored(t, WithGutter(markGutter{}))
	m, _ = keys(t, m, "j", "j", "j")
	for i, l := range strings.Split(plain(m), "\n")[:7] {
		if got := strings.HasPrefix(l, "> "); got != (i == 3) {
			t.Errorf("row %d: %q", i, l)
		}
	}
	off := fresh(t, WithSize(40, 8), WithGutter(markGutter{}))
	off.SetContent("f.txt", numbered(50))
	if strings.Contains(plain(off), ">") {
		t.Error("a pager without a cursor drew one")
	}
}

func TestNumberGutterMarksCursor(t *testing.T) {
	t.Parallel()
	on, off := cursored(t), fresh(t, WithSize(40, 8))
	off.SetContent("f.txt", numbered(50))
	if on.View() == off.View() {
		t.Error("the cursor's number looks like the others")
	}
}

package pager

import (
	"strings"
	"testing"
)

func TestBodyIsViewWithoutTheStatusLine(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithRows(hostRowsOf(50, "entry")))
	body, view := m.Body(), m.View()
	if !strings.HasPrefix(view, body) {
		t.Fatalf("view doesn't start with the body:\n%s\n---\n%s", view, body)
	}
	if got := strings.Count(body, "\n"); got != 7 {
		t.Errorf("body has %d rows, want 7", got)
	}
	if strings.Contains(view[len(body):], "\n") {
		t.Errorf("more than the status line follows the body: %q", view[len(body):])
	}
	if got := (Model{}).Body(); got != "" {
		t.Errorf("sizeless body = %q", got)
	}
}

func TestMovesScrollTheWindow(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithRows(hostRowsOf(50, "entry")))
	m.Down(3)
	if m.top != 3 {
		t.Errorf("Down: top %d, want 3", m.top)
	}
	m.Up(2)
	if m.top != 1 {
		t.Errorf("Up: top %d, want 1", m.top)
	}
	m.PageDown()
	if m.top != 8 {
		t.Errorf("PageDown: top %d, want 8", m.top)
	}
	m.PageUp()
	if m.top != 1 {
		t.Errorf("PageUp: top %d, want 1", m.top)
	}
	m.HalfPageDown()
	if m.top != 4 {
		t.Errorf("HalfPageDown: top %d, want 4", m.top)
	}
	m.HalfPageUp()
	if m.top != 1 {
		t.Errorf("HalfPageUp: top %d, want 1", m.top)
	}
	m.End()
	if m.top != 50-7 {
		t.Errorf("End: top %d, want %d", m.top, 50-7)
	}
	m.Down(10)
	if m.top != 50-7 {
		t.Errorf("Down past the end: top %d", m.top)
	}
	if got := m.bottom(); got != 49 {
		t.Errorf("bottom %d, want the last row", got)
	}
	m.Home()
	if m.top != 0 {
		t.Errorf("Home: top %d, want 0", m.top)
	}
}

func TestMovesMoveTheCursor(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithCursor(true), WithRows(hostRowsOf(50, "entry")))
	m.Down(1)
	if m.Cursor() != 1 {
		t.Errorf("Down: cursor %d, want 1", m.Cursor())
	}
	m.PageDown()
	if m.Cursor() != 8 {
		t.Errorf("PageDown: cursor %d, want 8", m.Cursor())
	}
	m.End()
	if m.Cursor() != 49 {
		t.Errorf("End: cursor %d, want 49", m.Cursor())
	}
	m.Up(1)
	if m.Cursor() != 48 {
		t.Errorf("Up: cursor %d, want 48", m.Cursor())
	}
	m.Home()
	if m.Cursor() != 0 || m.top != 0 {
		t.Errorf("Home: cursor %d top %d", m.Cursor(), m.top)
	}
	// The window keeps the cursor in view, so it goes there first.
	m.SetCursor(30)
	m.JumpTo(30)
	if !m.covers(30) {
		t.Errorf("JumpTo left line 30 out of view (top %d)", m.top)
	}
	m.SetCursor(45)
	m.Show()
	if !m.covers(45) {
		t.Errorf("Show left the cursor out of view (top %d)", m.top)
	}
}

func TestMovesScrollSideways(t *testing.T) {
	long := hostRows{strings.Repeat("abcdefghij", 10), "short"}
	m := fresh(t, WithSize(30, 5), WithRows(long))
	m.Right()
	if m.left == 0 {
		t.Fatal("Right didn't scroll")
	}
	before := m.left
	m.Left()
	if m.left >= before {
		t.Errorf("Left: %d, was %d", m.left, before)
	}
	m.Left()
	m.Left()
	if m.left != 0 {
		t.Errorf("Left past the start: %d", m.left)
	}
}

// A header row is drawn whole, so its length doesn't let the rest scroll.
func TestRightSkipsHeaderRows(t *testing.T) {
	rows := timedRows{strings.Repeat("header ", 20), "short"}
	m := fresh(t, WithSize(30, 5), WithRows(rows), WithHeaderWriter(bannerWriter))
	m.Right()
	if m.left != 0 {
		t.Errorf("scrolled %d columns for a long header", m.left)
	}
}

func TestRightKeepsToTheLinesInTheWindow(t *testing.T) {
	m := fresh(t, WithSize(30, 5), WithRows(hostRows{"short", "tiny"}))
	m.Right()
	if m.left != 0 {
		t.Errorf("scrolled %d columns past lines that fit", m.left)
	}
}

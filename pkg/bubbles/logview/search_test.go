package logview

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type stray struct{}

func TestUnrelatedMessageKeepsFoldClosed(t *testing.T) {
	for _, msg := range []tea.Msg{stray{}, tea.MouseClickMsg{}} {
		m := open(t, WithSize(80, 24))
		m.CollapseAll()
		m = find(t, m, "unused")
		if !isOpen(t, m, "golangci-lint") {
			t.Fatal("the fold of the match stayed closed")
		}
		m.CollapseAll()
		m, _ = m.Update(msg)
		if isOpen(t, m, "golangci-lint") {
			t.Errorf("%T opened the fold again", msg)
		}
	}
}

// bigSearchLog is a log past the pager's limit for searching in the
// foreground, with matches near its start and end.
func bigSearchLog() (texts []string, n int) {
	n = 6000
	texts = make([]string, n)
	for i := range texts {
		texts[i] = fmt.Sprintf("line %05d %s", i, strings.Repeat("x", 60))
	}
	texts[10] = "line hit first"
	texts[n-10] = "line hit middle"
	texts[n-1] = "line hit last"
	return texts, n
}

func TestBackgroundSearchLandsFromCursor(t *testing.T) {
	texts, n := bigSearchLog()
	m := view(t, plainLines(texts...), WithSize(80, 24))
	m, _ = keys(t, m, "G", "k", "k", "k", "k", "k")
	if got := cursorText(m); got != texts[n-6] {
		t.Fatalf("cursor = %q, want %q", got, texts[n-6])
	}
	m, _ = keys(t, m, "/")
	m = typeText(t, m, "hit")
	m, cmd := m.Update(press("enter"))
	if cmd == nil {
		t.Fatal("the search of a big log did not run in the background")
	}
	m, _ = m.Update(cmd())
	if got := cursorText(m); got != texts[n-1] {
		t.Errorf("cursor after the search = %q, want the first match after it, %q", got, texts[n-1])
	}
}

func TestBackgroundSearchKeepsCursorMovedMeanwhile(t *testing.T) {
	texts, n := bigSearchLog()
	m := view(t, plainLines(texts...), WithSize(80, 24))
	m, _ = keys(t, m, "/")
	m = typeText(t, m, "hit")
	m, cmd := m.Update(press("enter"))
	if cmd == nil {
		t.Fatal("the search of a big log did not run in the background")
	}
	m, _ = keys(t, m, "G", "k", "k", "k", "k", "k")
	want := cursorText(m)
	m, _ = m.Update(cmd())
	if got := cursorText(m); got != want {
		t.Errorf("cursor after the search = %q, want it to stay at %q", got, want)
	}
	if want != texts[n-6] {
		t.Fatalf("cursor = %q, want %q", want, texts[n-6])
	}
}

func TestStepFromCursorBeforeFirstJump(t *testing.T) {
	const n = 6000
	texts := make([]string, n)
	for i := range texts {
		texts[i] = fmt.Sprintf("line %05d %s", i, strings.Repeat("x", 60))
	}
	texts[10] = "line hit first"
	texts[n-10] = "line hit middle"
	texts[n-1] = "line hit last"
	m := view(t, plainLines(texts...), WithSize(80, 24))
	m, _ = keys(t, m, "/")
	m = typeText(t, m, "hit")
	m, cmd := m.Update(press("enter"))
	if cmd == nil {
		t.Fatal("the search of a big log did not run in the background")
	}
	// The user scrolls to the bottom while it runs, so it does not jump.
	m, _ = keys(t, m, "G")
	m, _ = m.Update(cmd())
	if _, ok := m.pg.Current(); ok {
		t.Fatal("the search jumped to a match after the user scrolled")
	}
	m, _ = keys(t, m, "n")
	if got := cursorText(m); got != texts[n-1] {
		t.Errorf("next match from the cursor = %q, want %q", got, texts[n-1])
	}
}

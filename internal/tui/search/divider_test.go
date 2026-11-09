package search

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

func dividedSection(t *testing.T, w, h int) (*Section, *ui.Dividers) {
	t.Helper()
	s := newSection(t, newFake(), w, h)
	d := &ui.Dividers{Step: 2}
	s.SetDividers(d)
	return s, d
}

func TestDividerMovesResults(t *testing.T) {
	t.Parallel()
	s, _ := dividedSection(t, 140, 40)
	if got := s.listOuter(); got != listWidth {
		t.Fatalf("start width %d, want %d", got, listWidth)
	}
	press(t, s, "ctrl+right")
	if got := s.listOuter(); got != listWidth+2 {
		t.Errorf("after right %d, want %d", got, listWidth+2)
	}
	press(t, s, "ctrl+left")
	press(t, s, "ctrl+left")
	if got := s.listOuter(); got != listWidth-2 {
		t.Errorf("after left %d, want %d", got, listWidth-2)
	}
	press(t, s, "=")
	if got := s.listOuter(); got != listWidth {
		t.Errorf("after reset %d, want %d", got, listWidth)
	}
}

func TestDividerStopsAtMinimum(t *testing.T) {
	t.Parallel()
	s, d := dividedSection(t, 120, 40)
	for range 40 {
		press(t, s, "ctrl+left")
	}
	if got := s.listOuter(); got != panes.MinWidth {
		t.Errorf("left limit %d, want %d", got, panes.MinWidth)
	}
	press(t, s, "ctrl+right")
	if got := s.listOuter(); got != panes.MinWidth+2 {
		t.Errorf("first press back %d, want %d", got, panes.MinWidth+2)
	}
	for range 80 {
		press(t, s, "ctrl+right")
	}
	if got := s.width - s.listOuter(); got != panes.MinWidth {
		t.Errorf("preview %d, want %d", got, panes.MinWidth)
	}
	if !d.Moved() {
		t.Error("dividers not moved")
	}
}

func TestDividerNarrowAndNil(t *testing.T) {
	t.Parallel()
	s, d := dividedSection(t, 80, 24)
	press(t, s, "ctrl+right")
	if d.Moved() {
		t.Error("a narrow page moved the divider")
	}
	n := newSection(t, newFake(), 140, 40)
	press(t, n, "ctrl+right")
	if got := n.listOuter(); got != listWidth {
		t.Errorf("without dividers %d, want %d", got, listWidth)
	}
}

func TestDividerSharedAndResized(t *testing.T) {
	t.Parallel()
	s, d := dividedSection(t, 140, 40)
	d.List.Move(10)
	s.layout()
	if got := s.listOuter(); got != listWidth+10 {
		t.Errorf("shared move %d, want %d", got, listWidth+10)
	}
	d.List.Move(100)
	s.SetSize(100, 40)
	if got := s.listOuter(); got != 100-panes.MinWidth {
		t.Errorf("resized %d, want %d", got, 100-panes.MinWidth)
	}
}

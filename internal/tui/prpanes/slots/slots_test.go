package slots

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

type host struct{ id, title string }

func forms(title string, _ panes.Box) []string { return []string{title} }

func TestFollowKeepsLastRow(t *testing.T) {
	t.Parallel()
	s := New([]Slot[*host]{
		{Context: "a"},
		{Context: "b", Hovered: func(h *host) (string, string, bool) { return h.id, h.title, h.id != "" }},
	}, forms)
	h := &host{id: "x", title: "Row x"}
	s.Follow(h)
	if got := s.Hover(2); got != (Hover{"x", "Row x"}) {
		t.Fatalf("hover %+v", got)
	}
	h.id = ""
	s.Follow(h)
	if got := s.Hover(2); got.ID != "x" {
		t.Errorf("a pane with no row left the hover %+v, want the last row", got)
	}
	if got := s.Hover(1); got != (Hover{}) {
		t.Errorf("a pane with no preview hovers %+v", got)
	}
}

func TestOffsetFollowsRow(t *testing.T) {
	t.Parallel()
	s := New([]Slot[*host]{{Hovered: func(h *host) (string, string, bool) { return h.id, h.title, true }}}, forms)
	h := &host{id: "a"}
	s.Follow(h)
	s.Previews().Set(Key{Pane: 1, ID: "a"}, 4)
	if got := s.Offset(1); got != 4 {
		t.Fatalf("offset %d, want 4", got)
	}
	h.id = "b"
	s.Follow(h)
	if got := s.Offset(1); got != 0 {
		t.Errorf("offset of another row %d, want 0", got)
	}
}

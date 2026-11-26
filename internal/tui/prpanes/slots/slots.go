// Package slots is what the panes of a conversation modal share: the table of
// what each pane is made of, the row its preview follows, and how far each
// row's preview is scrolled. The host type H, the modal, is a type parameter,
// so no modal's type is named here.
package slots

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// Slot is what one pane of a modal is made of: the context of its keys, and
// what it shows. Every field but Context and Content may be nil.
type Slot[H any] struct {
	Context string
	Content func(h H, b panes.Box) panes.Content
	// Hovered names the row of the pane's list under the cursor, and its text,
	// for the pane's preview. ok is false when no row has anything to preview,
	// and the preview then stays on the last row that had. A pane with no
	// preview has none. It may record the row it names for Preview to draw, and
	// does so only when ok.
	Hovered func(h H) (id, title string, ok bool)
	// Preview is the whole body of the pane's preview of the hovered row.
	Preview func(h H, b panes.Box) string
	// Info are the forms of the right-hand info of the preview's frame, such as
	// the state of the hovered row.
	Info func(h H, b panes.Box) []string
	// Press takes a key that the modal doesn't, while the pane's list has the
	// focus. ok is false for a key it has no use for.
	Press func(h H, k tea.KeyPressMsg) (cmd tea.Cmd, ok bool)
	// Title is the title of the preview's frame, in place of the hovered row's
	// text, for a preview that follows more than the row.
	Title func(h H, b panes.Box) string
	// PressPreview takes a key while the preview has the focus. A preview with
	// none only scrolls.
	PressPreview func(h H, k tea.KeyPressMsg) tea.Cmd
}

// Hover is the row of a list that a preview shows.
type Hover struct{ ID, Title string }

// Key names a preview: the row of a pane's list it shows.
type Key struct {
	Pane panes.Pane
	ID   string
}

// Forms gives the forms of a title that fits box b, the longest first.
type Forms func(title string, b panes.Box) []string

// Set is the slots of a modal, one per pane in the order of the panes, with
// the row each preview follows and the scroll of each row's preview. It is
// used through a pointer, since its maps and tables are shared by the
// modal's copies.
type Set[H any] struct {
	slots    []Slot[H]
	hover    []Hover
	previews panes.Scrolls[Key]
	forms    Forms
}

// New returns the set of slots, which it copies so that a modal may change
// its own. forms shapes the titles of the previews' frames.
func New[H any](slots []Slot[H], forms Forms) *Set[H] {
	return &Set[H]{
		slots: append([]Slot[H](nil), slots...),
		hover: make([]Hover, len(slots)),
		forms: forms,
	}
}

// Slot returns the slot of pane p.
func (s *Set[H]) Slot(p panes.Pane) *Slot[H] { return &s.slots[p-1] }

// Hover returns the row pane p's preview shows.
func (s *Set[H]) Hover(p panes.Pane) Hover { return s.hover[p-1] }

// Previews returns the scrolls of the previews.
func (s *Set[H]) Previews() *panes.Scrolls[Key] { return &s.previews }

// Follow moves the previews to the rows that the lists have under their
// cursors. A list with no row to preview leaves its preview where it was.
func (s *Set[H]) Follow(h H) {
	for i, sl := range s.slots {
		if sl.Hovered == nil {
			continue
		}
		if id, title, ok := sl.Hovered(h); ok {
			s.hover[i] = Hover{id, title}
		}
	}
}

// Offset is how far the preview of pane p is scrolled.
func (s *Set[H]) Offset(p panes.Pane) int {
	return s.previews.Get(Key{p, s.hover[p-1].ID})
}

// Content frames the preview in box b: its pane's body, scrolled to where
// this row was left, under the row's text.
func (s *Set[H]) Content(h H, b panes.Box) panes.Content {
	hv := s.hover[b.Pane-1]
	sl := s.slots[b.Pane-1]
	if sl.Preview == nil {
		return panes.Content{TopLeft: s.forms(hv.Title, b)}
	}
	body, _ := panes.Window(sl.Preview(h, b), s.previews.Get(Key{b.Pane, hv.ID}), b.Rect.Inner().H)
	title := hv.Title
	if sl.Title != nil {
		title = sl.Title(h, b)
	}
	c := panes.Content{TopLeft: s.forms(title, b), Body: body}
	if sl.Info != nil {
		c.TopRight = sl.Info(h, b)
	}
	return c
}

// Press gives a key to pane p's list, and reports what it asks for.
func (s *Set[H]) Press(h H, p panes.Pane, k tea.KeyPressMsg) tea.Cmd {
	if press := s.slots[p-1].Press; press != nil {
		cmd, _ := press(h, k)
		return cmd
	}
	return nil
}

// PressPreview gives a key to pane p's preview, which has the focus. ok is
// false when the slot has no keys of its own, and the key then scrolls.
func (s *Set[H]) PressPreview(h H, p panes.Pane, k tea.KeyPressMsg) (cmd tea.Cmd, ok bool) {
	if press := s.slots[p-1].PressPreview; press != nil {
		return press(h, k), true
	}
	return nil, false
}

// Scroll scrolls pane p's preview, shown in box b, as far as its text goes.
// The offset stays with the row shown.
func (s *Set[H]) Scroll(h H, p panes.Pane, b panes.Box, k ui.MoveKeys, msg tea.KeyPressMsg) {
	sl := s.slots[p-1]
	if sl.Preview == nil {
		return
	}
	rows := b.Rect.Inner().H
	last := max(0, strings.Count(sl.Preview(h, b), "\n")+1-rows)
	pk := Key{p, s.hover[p-1].ID}
	off := min(s.previews.Get(pk), last)
	switch {
	case keymap.Matches(msg, k.Up):
		off--
	case keymap.Matches(msg, k.Down):
		off++
	case keymap.Matches(msg, k.PageUp):
		off -= rows
	case keymap.Matches(msg, k.PageDown):
		off += rows
	case keymap.Matches(msg, k.HalfPageUp):
		off -= max(1, rows/2)
	case keymap.Matches(msg, k.HalfPageDown):
		off += max(1, rows/2)
	case keymap.Matches(msg, k.Home):
		off = 0
	case keymap.Matches(msg, k.End):
		off = last
	default:
		return
	}
	s.previews.Set(pk, min(max(off, 0), last))
}

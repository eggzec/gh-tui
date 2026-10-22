// Package panes lays out the numbered panes of a modal and draws their
// frames. It is pure: it takes sizes and caller-rendered content and returns
// rectangles and strings, and it handles no keys and holds no data.
//
// The caller supplies the panes (Spec.Panes) and decides what is focused and
// titled. The package knows one rule of its own: a zoomed layout shows the
// focused pane alone, or its list beside its preview. Any other arrangement
// is built by the caller out of the same Box and Layout types, so that the
// frames and overlays stay shared. Pane sizes are fixed: content scrolls
// inside a pane.
package panes

// maxPanes is how many panes a layout holds: a pane's digit is its position,
// and a digit is one key.
const maxPanes = 9

// Def describes one pane of a layout.
type Def struct {
	// Name is the pane's full name, such as "Conversation", and Short a
	// shorter one, such as "Conv", for a strip that has little room. Both are
	// the caller's words, so the bubble never has to know what the panes are.
	Name, Short string
	// Split is set for a pane that shows its list beside a preview when it is
	// zoomed and the area is wide enough. A pane with no sub-panes has
	// nothing to preview and stays whole.
	Split bool
}

// Pane is a pane's digit: its 1-based position in Spec.Panes. The zero Pane
// is no pane.
type Pane int

const (
	// DefaultSplitMin is the area width from which a zoomed list pane has a
	// preview beside it, when Spec.SplitMin is not set.
	DefaultSplitMin = 100
	// DefaultListWidth is the width of the list beside a preview, when
	// Spec.ListWidth is not set.
	DefaultListWidth = 46
)

// Rect is a rectangle of cells in the area, from its top left cell.
type Rect struct{ X, Y, W, H int }

// Inner is the rectangle for a box's content: inside the border and the one
// column of padding on each side. It is empty when the box is too small.
//
// It always assumes that padding. Frame leaves the padding out of a box
// under 5 cells wide, to give the content the room, so such a box shows up
// to 2 cells where Inner is empty or narrower; callers that fill a box that
// small are better off not drawing content in it.
func (r Rect) Inner() Rect {
	return Rect{X: r.X + 2, Y: r.Y + 1, W: max(0, r.W-4), H: max(0, r.H-2)}
}

// Role says what a box is for.
type Role int

// The roles of a box.
const (
	// Whole is a pane's only box.
	Whole Role = iota
	// List is the left box of a zoomed pane beside its preview.
	List
	// Preview is the right box of a zoomed pane beside its list.
	Preview
)

// Box is one framed rectangle of the layout.
type Box struct {
	Pane Pane
	Role Role
	// Rect is the whole box, border included. Use Rect.Inner for the content.
	Rect Rect
}

// Spec is what a zoomed layout depends on.
type Spec struct {
	// Width and Height are the area of the panes.
	Width, Height int
	// Panes are the panes, in digit order: at most 9, and the rest are
	// ignored.
	Panes []Def
	// Focus is the focused pane; a Focus that is no pane counts as the first.
	Focus Pane
	// Zoom lays the focus out alone, or beside its preview. Without it
	// Compute has no boxes to give, since arranging several panes is the
	// caller's: it builds a Layout from the same fields.
	Zoom bool
	// SplitMin is the area width from which a Split pane has a preview, and
	// ListWidth the width of its list; 0 is DefaultSplitMin and
	// DefaultListWidth.
	SplitMin, ListWidth int
}

// Layout is the boxes of an area, in the order they are drawn.
type Layout struct {
	Width, Height int
	// Zoomed is set when one pane fills the area, as Spec.Zoom asked.
	Zoomed bool
	// Focus is the focused pane, as Compute settled it, for the caller to
	// decide which box is drawn as focused.
	Focus Pane
	// Panes are the panes the layout was made for.
	Panes []Def
	Boxes []Box
}

// Compute lays out the area of s. Boxes never leave the area, and a box
// that does not fit has no cells.
func Compute(s Spec) Layout {
	w, h := max(0, s.Width), max(0, s.Height)
	defs := s.Panes[:min(len(s.Panes), maxPanes)]
	if s.Focus < 1 || int(s.Focus) > len(defs) {
		s.Focus = min(1, Pane(len(defs)))
	}
	l := Layout{Width: w, Height: h, Zoomed: s.Zoom, Focus: s.Focus, Panes: defs}
	if !s.Zoom || len(defs) == 0 {
		return l
	}
	splitMin, listW := s.SplitMin, s.ListWidth
	if splitMin <= 0 {
		splitMin = DefaultSplitMin
	}
	if listW <= 0 {
		listW = DefaultListWidth
	}
	if w < splitMin || !defs[s.Focus-1].Split {
		l.Boxes = []Box{{Pane: s.Focus, Rect: Rect{0, 0, w, h}}}
	} else {
		l.Boxes = []Box{
			{Pane: s.Focus, Role: List, Rect: Rect{0, 0, listW, h}},
			{Pane: s.Focus, Role: Preview, Rect: Rect{listW, 0, w - listW, h}},
		}
	}
	for i := range l.Boxes {
		l.Boxes[i].Rect = clamp(l.Boxes[i].Rect, w, h)
	}
	return l
}

// Box finds the box of a pane with a role.
func (l Layout) Box(p Pane, r Role) (Box, bool) {
	for _, b := range l.Boxes {
		if b.Pane == p && b.Role == r {
			return b, true
		}
	}
	return Box{}, false
}

// Title is the digit and the name of a pane, such as "2 Conversation": the
// usual left part of the pane's top border. It is empty for a Pane that is
// none of the layout's.
func (l Layout) Title(p Pane) string {
	if p < 1 || int(p) > len(l.Panes) {
		return ""
	}
	return string(rune('0'+p)) + " " + l.Panes[p-1].Name
}

// clamp cuts r to the area.
func clamp(r Rect, w, h int) Rect {
	r.X, r.Y = max(0, min(r.X, w)), max(0, min(r.Y, h))
	r.W, r.H = max(0, min(r.W, w-r.X)), max(0, min(r.H, h-r.Y))
	return r
}

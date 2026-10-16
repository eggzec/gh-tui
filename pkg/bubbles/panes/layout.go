// Package panes lays out the numbered panes of a modal and draws their
// frames. It is pure: it takes sizes and caller-rendered content and returns
// rectangles and strings, and it handles no keys and holds no data.
//
// The layout is two columns at 120 columns or more (Description above
// Conversation on the left, Merge, Involved, Development and Files on the
// right), or one zoomed pane, or a zoomed list beside its preview, with a
// pane strip on the bottom border. Pane sizes are fixed: content scrolls
// inside a pane, so the layout depends only on the area, the zoom state and
// the number of rows the Involved and Development panes have by default.
package panes

// Pane is one of the six panes. Its value is its digit, so the zero Pane is
// no pane.
type Pane int

// The panes, in digit order.
const (
	Description Pane = iota + 1
	Conversation
	Merge
	Involved
	Development
	Files
)

// Count is the number of panes.
const Count = int(Files)

var (
	names  = [...]string{"Description", "Conversation", "Merge", "Involved", "Development", "Files"}
	shorts = [...]string{"Desc", "Conv", "Merge", "Inv", "Dev", "Files"}
)

// Valid reports whether p is one of the six panes.
func (p Pane) Valid() bool { return p >= Description && p <= Files }

// Name is the pane's full name, such as "Conversation".
func (p Pane) Name() string {
	if !p.Valid() {
		return ""
	}
	return names[p-1]
}

// Short is the pane's short name, such as "Conv".
func (p Pane) Short() string {
	if !p.Valid() {
		return ""
	}
	return shorts[p-1]
}

// Title is the digit and the name, such as "2 Conversation": the left part
// of the pane's top border.
func (p Pane) Title() string {
	if !p.Valid() {
		return ""
	}
	return string(rune('0'+p)) + " " + p.Name()
}

// Sizes the layout is built on.
const (
	// WideMin is the area width from which the panes show in two columns.
	WideMin = 120
	// SplitMin is the area width from which a zoomed list pane has a
	// preview beside it.
	SplitMin = 100
	// SideWidth is the width of the right column of the wide layout.
	SideWidth = 36
	// ListWidth is the width of the list beside a preview.
	ListWidth = 46
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

// Spec is what a layout depends on.
type Spec struct {
	// Width and Height are the area of the panes: the modal body without its
	// header and footer.
	Width, Height int
	// Focus is the focused pane. Below WideMin it is the pane that fills the
	// area; a Focus that is no pane counts as Description.
	Focus Pane
	// Zoom zooms Focus at any width. Below WideMin the layout is zoomed
	// whatever Zoom says.
	Zoom bool
	// InvolvedRows and DevelopmentRows are the rows those panes have with
	// every fold in its default state, not its current state.
	InvolvedRows, DevelopmentRows int
}

// Layout is the boxes of an area, in the order they are drawn.
type Layout struct {
	Width, Height int
	// Zoomed is set when one pane fills the area (and carries the pane
	// strip), either because of Spec.Zoom or because the area is narrow.
	Zoomed bool
	// Focus is the focused pane, as Compute settled it. Draw draws the boxes
	// of this pane as focused; a Layout built by hand with no Focus has none.
	Focus Pane
	Boxes []Box
}

// Compute lays out the area of s. Boxes never leave the area, and a box
// that does not fit has no cells.
func Compute(s Spec) Layout {
	w, h := max(0, s.Width), max(0, s.Height)
	if !s.Focus.Valid() {
		s.Focus = Description
	}
	l := Layout{Width: w, Height: h, Zoomed: s.Zoom || w < WideMin, Focus: s.Focus}
	if l.Zoomed {
		l.Boxes = zoomed(s.Focus, w, h)
	} else {
		l.Boxes = wide(s, w, h)
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

// zoomed is one pane over the whole area, or a list beside its preview for
// the panes below Merge's number. Description and Conversation never split,
// deliberately: they have no sub-panes, so there is nothing to preview.
func zoomed(p Pane, w, h int) []Box {
	if w < SplitMin || p < Merge {
		return []Box{{Pane: p, Rect: Rect{0, 0, w, h}}}
	}
	return []Box{
		{Pane: p, Role: List, Rect: Rect{0, 0, ListWidth, h}},
		{Pane: p, Role: Preview, Rect: Rect{ListWidth, 0, w - ListWidth, h}},
	}
}

func wide(s Spec, w, h int) []Box {
	left := w - SideWidth
	desc := descriptionHeight(h)
	merge, inv, dev, files := rightHeights(h, s.InvolvedRows, s.DevelopmentRows)
	boxes := make([]Box, 0, 6)
	boxes = append(boxes,
		Box{Pane: Description, Rect: Rect{0, 0, left, desc}},
		Box{Pane: Conversation, Rect: Rect{0, desc, left, h - desc}},
	)
	y := 0
	for _, c := range []struct {
		p Pane
		h int
	}{{Merge, merge}, {Involved, inv}, {Development, dev}, {Files, files}} {
		boxes = append(boxes, Box{Pane: c.p, Rect: Rect{left, y, SideWidth, c.h}})
		y += c.h
	}
	return boxes
}

func descriptionHeight(h int) int {
	return max(0, min(max(5, h/3), max(3, h-4)))
}

// rightHeights are the heights of the right column. Merge, Involved and
// Development shrink one line at a time, tallest first (Merge winning ties),
// while their sum is more than h-5, and never below 3 lines each. Files gets
// what is left, but at least 3 lines: so at least 5 whenever the three above
// can shrink that far, and fewer only when h is under 14.
func rightHeights(h, invRows, devRows int) (merge, inv, dev, files int) {
	hs := [3]int{11, max(4, invRows+2), max(3, devRows+2)}
	for hs[0]+hs[1]+hs[2] > h-5 {
		t := -1
		for i, v := range hs {
			if v > 3 && (t < 0 || v > hs[t]) {
				t = i
			}
		}
		if t < 0 {
			break
		}
		hs[t]--
	}
	files = max(3, h-(hs[0]+hs[1]+hs[2]))
	return hs[0], hs[1], hs[2], files
}

// clamp cuts r to the area.
func clamp(r Rect, w, h int) Rect {
	r.X, r.Y = max(0, min(r.X, w)), max(0, min(r.Y, h))
	r.W, r.H = max(0, min(r.W, w-r.X)), max(0, min(r.H, h-r.Y))
	return r
}

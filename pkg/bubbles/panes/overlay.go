package panes

// OverlayKind is a kind of overlay opened over the panes.
type OverlayKind int

// The kinds of overlay.
const (
	// Viewer is a diff, check or help view: inset 4 columns and 2 lines on a
	// wide area.
	Viewer OverlayKind = iota
	// Picker is a list to choose from, centred on a wide area.
	Picker
	// Confirm is a three-line prompt, centred at any width.
	Confirm
)

// The most a wide picker takes.
const (
	pickerWidth  = 46
	pickerHeight = 14
)

// Overlay describes an overlay to place.
type Overlay struct {
	Kind OverlayKind
	// Items is the number of rows of a Picker.
	Items int
	// Text is the width of the text of a Confirm.
	Text int
	// Maximized makes a Viewer fill the area even when it is wide.
	Maximized bool
}

// OverlayRect places an overlay in an area of w by h cells, the panes' area
// without the footer. Viewers and pickers fill the area when it is narrower
// than WideMin, and viewers also when maximized. The rectangle always lies
// inside the area.
func OverlayRect(o Overlay, w, h int) Rect {
	w, h = max(0, w), max(0, h)
	full := Rect{0, 0, w, h}
	wide := w >= WideMin
	var r Rect
	switch o.Kind {
	case Viewer:
		if !wide || o.Maximized {
			return full
		}
		r = Rect{4, 2, w - 8, h - 4}
	case Picker:
		if !wide {
			return full
		}
		r = centred(min(pickerWidth, w-4), min(o.Items+4, pickerHeight), w, h)
	case Confirm:
		r = centred(min(w-2, max(40, o.Text+8)), 3, w, h)
	}
	return clamp(r, w, h)
}

func centred(bw, bh, w, h int) Rect {
	bw, bh = max(0, bw), max(0, min(bh, h))
	return Rect{(w - bw) / 2, (h - bh) / 2, bw, bh}
}

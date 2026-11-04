package issues

import "github.com/eggzec/gh-tui/pkg/bubbles/panes"

// The panes of the issue modal, in digit order.
const (
	paneDescription panes.Pane = iota + 1
	paneConversation
	paneInvolved
	paneDevelopment
	paneBlockers
)

// paneCount is the number of panes.
const paneCount = int(paneBlockers)

// paneDefs are the panes as the panes bubble takes them. The panes from
// Involved on show a list beside a preview when zoomed; Description and
// Conversation have no sub-panes, so there is nothing to preview.
var paneDefs = []panes.Def{
	{Name: "Description", Short: "Desc"},
	{Name: "Conversation", Short: "Conv"},
	{Name: "Involved", Short: "Inv", Split: true},
	{Name: "Development", Short: "Dev", Split: true},
	{Name: "Blockers", Short: "Block", Split: true},
}

// Sizes the issue layout is built on.
const (
	// wideMin is the area width from which the panes show in two columns.
	wideMin = 120
	// sideWidth is the width of the right column of the wide layout.
	sideWidth = 36
)

// showsOne reports whether an area w cells wide shows the focused pane alone:
// when zoom asks for it, and always under wideMin.
func showsOne(w int, zoom bool) bool { return zoom || w < wideMin }

// paneName is the pane's full name, such as "Conversation".
func paneName(p panes.Pane) string {
	if p < 1 || int(p) > len(paneDefs) {
		return ""
	}
	return paneDefs[p-1].Name
}

// paneTitle is the digit and the name, such as "2 Conversation".
func paneTitle(p panes.Pane) string {
	return panes.Layout{Panes: paneDefs}.Title(p)
}

// issueLayout lays out an area of w by h cells: two columns from wideMin
// columns, unless zoom asks for the focus alone, and otherwise the zoomed
// layout of the bubble.
func issueLayout(w, h int, focus panes.Pane, zoom bool) panes.Layout {
	spec := panes.Spec{Width: w, Height: h, Panes: paneDefs, Focus: focus, Zoom: showsOne(w, zoom)}
	if spec.Zoom {
		return panes.Compute(spec)
	}
	w, h = max(0, w), max(0, h)
	if focus < 1 || int(focus) > paneCount {
		focus = paneDescription
	}
	l := panes.Layout{Width: w, Height: h, Focus: focus, Panes: paneDefs, Boxes: wideBoxes(w, h)}
	for i := range l.Boxes {
		l.Boxes[i].Rect = clampRect(l.Boxes[i].Rect, w, h)
	}
	return l
}

func wideBoxes(w, h int) []panes.Box {
	left := w - sideWidth
	desc := descriptionHeight(h)
	// The right column is three stacked thirds; the last takes the rest.
	third := h / 3
	return []panes.Box{
		{Pane: paneDescription, Rect: panes.Rect{X: 0, Y: 0, W: left, H: desc}},
		{Pane: paneConversation, Rect: panes.Rect{X: 0, Y: desc, W: left, H: h - desc}},
		{Pane: paneInvolved, Rect: panes.Rect{X: left, Y: 0, W: sideWidth, H: third}},
		{Pane: paneDevelopment, Rect: panes.Rect{X: left, Y: third, W: sideWidth, H: third}},
		{Pane: paneBlockers, Rect: panes.Rect{X: left, Y: 2 * third, W: sideWidth, H: h - 2*third}},
	}
}

func descriptionHeight(h int) int {
	return max(0, min(max(5, h/3), max(3, h-4)))
}

// clampRect cuts r to the area.
func clampRect(r panes.Rect, w, h int) panes.Rect {
	r.X, r.Y = max(0, min(r.X, w)), max(0, min(r.Y, h))
	r.W, r.H = max(0, min(r.W, w-r.X)), max(0, min(r.H, h-r.Y))
	return r
}

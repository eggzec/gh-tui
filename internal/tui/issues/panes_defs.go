package issues

import (
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

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

// issueLayout lays out an area of w by h cells: two columns from wideMin
// columns, unless zoom asks for the focus alone, and otherwise the zoomed
// layout of the bubble.
func issueLayout(w, h int, focus panes.Pane, zoom bool, d *ui.Dividers) panes.Layout {
	spec := panes.Spec{Width: w, Height: h, Panes: paneDefs, Focus: focus, Zoom: showsOne(w, zoom), ListWidth: listSplit(d, w)}
	if spec.Zoom {
		return panes.Compute(spec)
	}
	w, h = max(0, w), max(0, h)
	if focus < 1 || int(focus) > paneCount {
		focus = paneDescription
	}
	first, second := infoCuts(d, h)
	l := panes.Layout{Width: w, Height: h, Focus: focus, Panes: paneDefs, Boxes: wideBoxes(w, h, sideSplit(d, w), descSplit(d, h), first, second)}
	for i := range l.Boxes {
		l.Boxes[i].Rect = clampRect(l.Boxes[i].Rect, w, h)
	}
	return l
}

// sideSplit is the width of the side column: sideWidth until the user moves
// the divider, which keeps MinWidth columns on each side.
func sideSplit(d *ui.Dividers, w int) int {
	if d == nil {
		return sideWidth
	}
	return w - d.Side.Of(w-sideWidth, panes.MinWidth, w-panes.MinWidth)
}

// listSplit is the width of the list beside the preview of a zoomed pane, or 0
// for the bubble's default.
func listSplit(d *ui.Dividers, w int) int {
	if d == nil {
		return 0
	}
	return d.List.Of(panes.DefaultListWidth, panes.MinWidth, w-panes.MinWidth)
}

// descSplit is the height of the Description in an area h high, where d may
// have moved it from descriptionHeight; a nil d leaves it.
func descSplit(d *ui.Dividers, h int) int {
	if d == nil {
		return descriptionHeight(h)
	}
	return d.Desc.Of(descriptionHeight(h), panes.MinHeight, h-panes.MinHeight)
}

// infoCuts are the rows where the side column's panes start below the first:
// the thirds of an h high column until the user moves a divider. Each cut keeps
// MinHeight rows for the pane above it and for every pane below.
func infoCuts(d *ui.Dividers, h int) (first, second int) {
	first, second = h/3, 2*(h/3)
	if d == nil || h < 3*panes.MinHeight {
		return first, second
	}
	first = d.Info[0].Of(first, panes.MinHeight, h-2*panes.MinHeight)
	second = d.Info[1].Of(second, first+panes.MinHeight, h-panes.MinHeight)
	return first, second
}

func wideBoxes(w, h, side, desc, first, second int) []panes.Box {
	left := w - side
	return []panes.Box{
		{Pane: paneDescription, Rect: panes.Rect{X: 0, Y: 0, W: left, H: desc}},
		{Pane: paneConversation, Rect: panes.Rect{X: 0, Y: desc, W: left, H: h - desc}},
		{Pane: paneInvolved, Rect: panes.Rect{X: left, Y: 0, W: side, H: first}},
		{Pane: paneDevelopment, Rect: panes.Rect{X: left, Y: first, W: side, H: second - first}},
		{Pane: paneBlockers, Rect: panes.Rect{X: left, Y: second, W: side, H: h - second}},
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

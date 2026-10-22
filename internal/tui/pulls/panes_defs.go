package pulls

import "github.com/eggzec/gh-tui/pkg/bubbles/panes"

// The panes of the pull request modal, in digit order.
const (
	paneDescription panes.Pane = iota + 1
	paneConversation
	paneMerge
	paneInvolved
	paneDevelopment
	paneFiles
)

// paneCount is the number of panes.
const paneCount = int(paneFiles)

// paneDefs are the panes as the panes bubble takes them. The panes from Merge
// on show a list beside a preview when zoomed; Description and Conversation
// have no sub-panes, so there is nothing to preview.
var paneDefs = []panes.Def{
	{Name: "Description", Short: "Desc"},
	{Name: "Conversation", Short: "Conv"},
	{Name: "Merge", Short: "Merge", Split: true},
	{Name: "Involved", Short: "Inv", Split: true},
	{Name: "Development", Short: "Dev", Split: true},
	{Name: "Files", Short: "Files", Split: true},
}

// Sizes the pull request layout is built on.
const (
	// wideMin is the area width from which the panes show in two columns.
	wideMin = 120
	// sideWidth is the width of the right column of the wide layout.
	sideWidth = 36
)

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

// prLayout lays out an area of w by h cells: two columns from wideMin
// columns, unless zoom asks for the focus alone, and otherwise the zoomed
// layout of the bubble. devRows is the rows the Development pane has with
// every fold in its default state, not its current one.
func prLayout(w, h int, focus panes.Pane, zoom bool, devRows int) panes.Layout {
	spec := panes.Spec{Width: w, Height: h, Panes: paneDefs, Focus: focus, Zoom: zoom || w < wideMin}
	if spec.Zoom {
		return panes.Compute(spec)
	}
	w, h = max(0, w), max(0, h)
	if focus < 1 || int(focus) > paneCount {
		focus = paneDescription
	}
	l := panes.Layout{Width: w, Height: h, Focus: focus, Panes: paneDefs, Boxes: wideBoxes(w, h, devRows)}
	for i := range l.Boxes {
		l.Boxes[i].Rect = clampRect(l.Boxes[i].Rect, w, h)
	}
	return l
}

func wideBoxes(w, h, devRows int) []panes.Box {
	left := w - sideWidth
	desc := descriptionHeight(h)
	merge, inv, dev, files := rightHeights(h, 0, devRows)
	boxes := make([]panes.Box, 0, paneCount)
	boxes = append(boxes,
		panes.Box{Pane: paneDescription, Rect: panes.Rect{X: 0, Y: 0, W: left, H: desc}},
		panes.Box{Pane: paneConversation, Rect: panes.Rect{X: 0, Y: desc, W: left, H: h - desc}},
	)
	y := 0
	for _, c := range []struct {
		p panes.Pane
		h int
	}{{paneMerge, merge}, {paneInvolved, inv}, {paneDevelopment, dev}, {paneFiles, files}} {
		boxes = append(boxes, panes.Box{Pane: c.p, Rect: panes.Rect{X: left, Y: y, W: sideWidth, H: c.h}})
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

// clampRect cuts r to the area.
func clampRect(r panes.Rect, w, h int) panes.Rect {
	r.X, r.Y = max(0, min(r.X, w)), max(0, min(r.Y, h))
	r.W, r.H = max(0, min(r.W, w-r.X)), max(0, min(r.H, h-r.Y))
	return r
}

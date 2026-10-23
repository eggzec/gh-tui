package actions

import "github.com/eggzec/gh-tui/pkg/bubbles/panes"

// The panes of the modal, in digit order.
const (
	paneRuns panes.Pane = iota + 1
	paneNotes
)

// paneCount is the number of panes.
const paneCount = int(paneNotes)

// paneDefs are the panes as the panes bubble takes them. Neither has
// sub-panes to preview.
var paneDefs = []panes.Def{
	{Name: "Runs", Short: "Runs"},
	{Name: "Annotations", Short: "Annot."},
}

// Sizes the layout is built on.
const (
	// wideMin is the area width from which both panes show.
	wideMin = 100
	// runsWidth is the width of the runs pane in the wide layout.
	runsWidth = 34
)

// showsOne reports whether an area w cells wide shows the focused pane alone:
// when zoom asks for it, and always under wideMin.
func showsOne(w int, zoom bool) bool { return zoom || w < wideMin }

// paneName is the pane's full name, such as "Runs".
func paneName(p panes.Pane) string {
	if p < 1 || int(p) > len(paneDefs) {
		return ""
	}
	return paneDefs[p-1].Name
}

// paneLayout lays out an area of w by h cells: the runs beside the
// annotations from wideMin columns, unless zoom asks for the focus alone,
// and otherwise the zoomed layout of the bubble.
func paneLayout(w, h int, focus panes.Pane, zoom bool) panes.Layout {
	spec := panes.Spec{Width: w, Height: h, Panes: paneDefs, Focus: focus, Zoom: showsOne(w, zoom)}
	if spec.Zoom {
		return panes.Compute(spec)
	}
	w, h = max(0, w), max(0, h)
	if focus < 1 || int(focus) > paneCount {
		focus = paneRuns
	}
	left := min(runsWidth, w)
	return panes.Layout{Width: w, Height: h, Focus: focus, Panes: paneDefs, Boxes: []panes.Box{
		{Pane: paneRuns, Rect: panes.Rect{W: left, H: h}},
		{Pane: paneNotes, Rect: panes.Rect{X: left, W: w - left, H: h}},
	}}
}

package panes

// Controller owns the focus and the zoom of a set of panes, so every screen
// that shows them follows one rule:
//
//   - The digits always switch pane, zoomed or not.
//   - Tab and Shift+Tab cycle the panes only while unzoomed.
//   - Zoomed, Tab and Shift+Tab flip between the list and the preview of the
//     focused pane; a pane with no split does nothing.
//
// It is key-agnostic: a host maps its keys to Toggle, UndoZoom, Tab and Digit,
// and runs its own focus side effects when they report a change. The fields
// are exported so a host can read Zoom.Zoomed and Nav.InPreview.
type Controller struct {
	Nav  Nav
	Zoom Zoom
}

// Toggle zooms or unzooms the focused pane and gives every list the focus. It
// does nothing, and reports false, while the screen is too narrow to zoom.
func (c *Controller) Toggle() bool {
	if !c.Zoom.CanZoom() {
		return false
	}
	c.Zoom.Toggle()
	c.Nav.Reset()
	return true
}

// UndoZoom drops a zoom the user asked for, as Backspace does, and reports
// whether it did. A narrow screen has no user zoom to drop.
func (c *Controller) UndoZoom() bool {
	if !c.Zoom.User() || !c.Zoom.CanZoom() {
		return false
	}
	c.Zoom.SetZoom(false)
	c.Nav.Reset()
	return true
}

// SetNarrow records whether the screen is too narrow to zoom. It changes the
// layout, so the host calls it, rebuilds its layout from the controller's
// zoom, then calls Settle with that layout and resets its own previews when
// Settle returns true.
func (c *Controller) SetNarrow(narrow bool) {
	c.Zoom.SetNarrow(narrow)
}

// Settle gives the lists the focus again unless l still shows the focused
// pane's preview, so a preview that a resize took away does not keep the
// focus. It reports whether it did, so the host can reset its previews.
func (c *Controller) Settle(l Layout) bool {
	if c.Nav.InPreview(l) {
		return false
	}
	c.Nav.Reset()
	return true
}

// Tab steps by delta and returns the focused pane and whether anything
// changed. Unzoomed it moves to the next or previous pane of l, wrapping.
// Zoomed it flips the list and the preview of the focused pane, which keeps
// the focus, and does nothing when that pane has no preview. l must be the
// layout built from the controller's zoom.
func (c *Controller) Tab(l Layout, delta int) (Pane, bool) {
	if l.Zoomed {
		if !c.Nav.CanStep(l) {
			return c.Nav.Focus(), false
		}
		c.Nav.Step(l, delta)
		return c.Nav.Focus(), true
	}
	before := c.Nav.Focus()
	c.Nav.Step(l, delta)
	return c.Nav.Focus(), c.Nav.Focus() != before
}

// Digit focuses pane p, zoomed or not, and reports whether it did. A pane
// outside l is ignored. The zoom stays as it is. The pane's list gets the
// focus, even on the pane that already has it, so a zoomed pane showing its
// preview goes back to its list.
func (c *Controller) Digit(l Layout, p Pane) bool {
	if p < 1 || int(p) > len(l.Panes) {
		return false
	}
	c.Nav.SetFocus(p)
	return true
}

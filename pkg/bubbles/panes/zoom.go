package panes

// Zoom is whether a screen shows its focused pane alone. The user asks for it
// with a key, and a terminal too narrow for every pane forces it. The user's
// choice survives a narrow phase, so growing the terminal again brings back
// what they had. The zero value is not zoomed and not narrow.
type Zoom struct {
	user, narrow bool
}

// Zoomed reports whether the focused pane shows alone, for either reason.
func (z Zoom) Zoomed() bool { return z.user || z.narrow }

// User reports whether the user zoomed in. The back key undoes only that.
func (z Zoom) User() bool { return z.user }

// CanZoom reports whether the zoom key does anything: not while the narrow
// terminal forces the zoom.
func (z Zoom) CanZoom() bool { return !z.narrow }

// SetNarrow sets whether the terminal is too narrow for every pane. Whoever
// knows the terminal's size calls it.
func (z *Zoom) SetNarrow(narrow bool) { z.narrow = narrow }

// SetZoom sets the user's zoom. It does nothing while the terminal is narrow.
func (z *Zoom) SetZoom(zoom bool) {
	if z.narrow {
		return
	}
	z.user = zoom
}

// Toggle flips the user's zoom. It does nothing while the terminal is narrow.
func (z *Zoom) Toggle() { z.SetZoom(!z.user) }

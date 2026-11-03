package prpanes

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// Overlay is a picker, a confirm or a viewer that a modal opens over its
// panes. While one is open, the keys go to it alone, and the panes behind it
// keep their focus, zoom and scroll.
type Overlay interface {
	// Title is what the frame of the overlay says.
	Title() string
	// Kind is how the overlay is placed over the panes.
	Kind() panes.OverlayKind
	// Items is the number of rows of a picker, and Text the width of what a
	// confirm asks.
	Items() int
	Text() int
	// Update takes a key or a paste.
	Update(tea.Msg) tea.Cmd
	// View draws the inside of the frame, w by h cells.
	View(w, h int) string
	// Layers are the keys of the overlay, for the footer and the help.
	Layers() []keyhelp.Layer
	// Clear drops what is transient, such as a search, and reports whether
	// there was any.
	Clear() bool
	// Closed reports that the overlay is done and takes no more keys.
	Closed() bool
}

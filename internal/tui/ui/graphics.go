package ui

import "github.com/eggzec/gh-tui/internal/imgcaps"

// Graphics is whether the terminal shows the images the app draws, which
// the app finds out soon after it starts. Until then, and wherever it
// can't find out, no image is drawn.
type Graphics struct {
	// Images says to draw images, with kitty's Unicode placeholders.
	Images bool
	// Animate says the terminal plays an animation itself, once sent all
	// its frames, as kitty does.
	Animate bool
	// Tmux says that what is sent of an image goes through tmux, wrapped
	// in its passthrough.
	Tmux bool
	// Shared says images are off only because tmux shows the session on
	// more than one terminal, while the one they were drawn on, if any,
	// still holds them.
	Shared bool
	// Cell is the size of a cell in pixels, which an image is scaled by
	// to cover whole cells. It is zero until the app found it out, soon
	// after it found that the terminal draws images, and it changes when
	// the font or the terminal does.
	Cell imgcaps.Cell
}

// GraphicsMsg tells every section what the terminal shows, once the app
// found out, and again whenever that or the size of a cell changes.
type GraphicsMsg struct {
	Graphics Graphics
}

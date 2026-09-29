package ui

// Graphics is whether the terminal shows the images the app draws, which
// the app finds out soon after it starts. Until then, and wherever it
// can't find out, no image is drawn.
type Graphics struct {
	// Images says to draw images, with kitty's Unicode placeholders.
	Images bool
	// Tmux says that what is sent of an image goes through tmux, wrapped
	// in its passthrough.
	Tmux bool
}

// GraphicsMsg tells every section what the terminal shows, once the app
// found out.
type GraphicsMsg struct {
	Graphics Graphics
}

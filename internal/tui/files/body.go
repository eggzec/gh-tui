package files

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// fileBody shows the content of one file in a pager, as the file preview
// does: its image where it is an image file the terminal shows, and its
// markdown rendered unless the section shows the source. It reads nothing
// itself: its owner gets the blob and hands it to show.
type fileBody struct {
	pager pager.Model
	// img shows the file in the pager as its image, where it is one.
	img fileImage
	// md renders the markdown files shown.
	md markdownView
	// glyphs gives the glyphs of the section now, and raw says whether it
	// shows markdown as its source. Both are read at each use, since the
	// section changes them while the body lives.
	glyphs func() ui.Icons
	raw    func() bool
}

// drawsImage reports whether the body draws e as an image, so its owner
// reads e although its name says it is binary.
func (b *fileBody) drawsImage(e core.TreeEntry) bool {
	return !e.Symlink() && imageFile(e.Path) && b.img.images.Drawing()
}

// show shows e, whose content is blob, or which failed to load with err.
func (b *fileBody) show(e core.TreeEntry, blob core.Blob, err error) tea.Cmd {
	b.img.set(e, blob, err)
	if !b.img.draw(&b.pager) {
		return nil
	}
	if text, ok := ui.OpenToLoadText(e.Path, err); ok {
		b.pager.SetMessage(e.Path, text)
		return nil
	}
	cmd, _ := fill(&b.pager, e, blob, err, b.glyphs(), b.rendering())
	return tea.Batch(cmd, b.md.lookUp())
}

// clear shows no file: its image goes, and the pager shows e's name above
// text, which may be empty.
func (b *fileBody) clear(e, text string) {
	b.img.clear()
	b.pager.SetMessage(e, text)
}

// rendering returns what renders the markdown files shown, or nil while
// the section shows them as their source.
func (b *fileBody) rendering() *markdownView {
	if b.raw() {
		return nil
	}
	return &b.md
}

// redraw draws the image of the file shown again, as when the images or
// the size of the body changed.
func (b *fileBody) redraw() tea.Cmd {
	if !b.img.redraw(&b.pager) {
		return nil
	}
	cmd, _ := fill(&b.pager, b.img.entry, b.img.blob, b.img.err, b.glyphs(), b.rendering())
	return cmd
}

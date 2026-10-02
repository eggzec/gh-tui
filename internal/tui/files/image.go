package files

import (
	"path"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// imageExts are the extensions of the files the previews try to draw as
// images. Whether a file is one comes from its content; the name only
// says which files to try, so a file of another name is never decoded.
var imageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

// imageFile reports whether the file at name is one to draw as an image.
func imageFile(name string) bool {
	return slices.Contains(imageExts, strings.ToLower(path.Ext(name)))
}

// shown is what a pager that shows a file shows of it.
type shown int

const (
	shownNothing shown = iota
	// shownText is the pager's own view of the file: its text, or why it
	// isn't shown.
	shownText
	// shownLoading says the image is on its way.
	shownLoading
	// shownImage is the image.
	shownImage
)

// fileImage shows a file of a repository in a pager: as its image, in
// the room above the pager's status line, while it is an image file the
// terminal shows, and otherwise as the pager shows any file. The file
// preview and the finder's preview share it, so both draw a file alike.
type fileImage struct {
	images *ui.Images
	repo   core.RepoRef
	// entry, blob and err are the file shown, its content and why it
	// failed to load, if it did. on is set while the file is one to draw
	// as an image, whether it is drawn, on its way or failed.
	entry core.TreeEntry
	blob  core.Blob
	err   error
	on    bool
	// shown is what the pager shows, and pic the lines of the image.
	shown shown
	pic   []string
	// ellipsis ends the text shown while the image loads.
	ellipsis string
}

// set takes the file the pager shows from now: e, whose content is b, or
// which failed to load with err.
func (fi *fileImage) set(e core.TreeEntry, b core.Blob, err error) {
	fi.entry, fi.blob, fi.err = e, b, err
	fi.on = err == nil && fi.images != nil && !e.Symlink() && imageFile(e.Path)
	fi.shown, fi.pic = shownNothing, nil
}

// clear forgets the file, for a pager that shows something else.
func (fi *fileImage) clear() {
	fi.set(core.TreeEntry{}, core.Blob{}, nil)
	fi.shown = shownText
}

// draw shows the file in pg as its image, or says the image is on its
// way, and reports whether pg must show the file as it shows any file
// instead, which it does only when what it shows changes, so drawing
// again keeps the place in the text.
func (fi *fileImage) draw(pg *pager.Model) (text bool) {
	rows, st := fi.rows(pg)
	want := shownText
	switch st {
	case ui.ImageShown:
		want = shownImage
	case ui.ImageLoading:
		want = shownLoading
	case ui.ImageFailed, ui.ImageOff:
		// It shows as it did before images, such as a binary file. A
		// failure that may mend is asked for again once GitHub answers
		// again, which draws the pager again.
	}
	fi.pic = rows
	if want == fi.shown {
		return false
	}
	fi.shown = want
	switch want {
	case shownImage:
		pg.SetMessage(fi.entry.Path, "")
	case shownLoading:
		pg.SetMessage(fi.entry.Path, "Loading the image"+fi.ellipsis)
	case shownNothing, shownText:
		return true
	}
	return false
}

// redraw draws the file again, as draw does, if it is an image file, as
// when the images or the pager's size changed. A file shown as text stays
// as it is, since its text is the pager's.
func (fi *fileImage) redraw(pg *pager.Model) (text bool) {
	return fi.on && fi.draw(pg)
}

// rows returns the lines of the image of the file, fitted to the room
// of pg above its status line, and how far the image got.
func (fi *fileImage) rows(pg *pager.Model) ([]string, ui.ImageState) {
	if !fi.on {
		return nil, ui.ImageOff
	}
	w, h := pg.Width(), pg.Height()-1
	if w <= 0 || h <= 0 {
		if fi.images.Drawing() {
			return nil, ui.ImageLoading
		}
		return nil, ui.ImageOff
	}
	src := ui.ImageSource{Repo: fi.repo, SHA: fi.entry.SHA, Size: fi.blob.Size}
	return fi.images.Fit(src, ui.ImageSize{Cols: w, Rows: h})
}

// view renders pg, or the image above its status line. The lines of the
// image reach the terminal as they are, in no style.
func (fi *fileImage) view(pg *pager.Model) string {
	v := pg.View()
	if fi.shown != shownImage {
		return v
	}
	w, h := pg.Width(), pg.Height()-1
	var b strings.Builder
	for i := range h {
		line := ""
		if i < len(fi.pic) {
			line = fi.pic[i]
		}
		if ansi.StringWidth(line) > w {
			line = ansi.Truncate(line, w, "")
		}
		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", w-ansi.StringWidth(line)))
		b.WriteByte('\n')
	}
	b.WriteString(v[strings.LastIndexByte(v, '\n')+1:])
	return b.String()
}

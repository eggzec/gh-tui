package files

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// fileIcons renders the icons of files in a theme. A repository holds
// thousands of files but few kinds of them, so each icon is rendered once.
type fileIcons struct {
	icons    ui.Icons
	theme    ui.Theme
	rendered map[ui.FileIcon]string
}

func newFileIcons(icons ui.Icons, t ui.Theme) *fileIcons {
	return &fileIcons{icons: icons, theme: t, rendered: map[ui.FileIcon]string{}}
}

// entry returns the rendered icon of e, open or not, or "" in icon sets
// without file icons.
func (f *fileIcons) entry(e core.TreeEntry, open bool) string {
	fi := f.icons.Entry(e, open)
	if fi.Glyph == "" {
		return ""
	}
	r, ok := f.rendered[fi]
	if !ok {
		r = f.theme.FileIcon(fi).Render(fi.Glyph)
		f.rendered[fi] = r
	}
	return r
}

// node is a tree.Icons.
func (f *fileIcons) node(n tree.Node, expanded bool) string {
	e, ok := entryOf(n)
	if !ok {
		return ""
	}
	return f.entry(e, expanded)
}

// item is a finder.Icons.
func (f *fileIcons) item(it finder.Item) string {
	e, ok := entryOfItem(it)
	if !ok {
		return ""
	}
	return f.entry(e, false)
}

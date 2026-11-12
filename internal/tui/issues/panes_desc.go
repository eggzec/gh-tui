package issues

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/desc"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// descPane is the Description pane: who opened the issue and its text, as the
// rows of a tree so that a body taller than the box scrolls and its details
// blocks fold. The rows are made again only when the text, the width or the
// look changes.
type descPane struct {
	tree  treeview.Model
	built descKey
	made  bool
	// look counts the changes of the theme and the icons.
	look int
	// pics counts the changes of the images.
	pics int
}

// descKey is what the rows of the description depend on.
type descKey struct {
	text  string
	width int
	look  int
	// pics counts the changes of the images, and picRows is the tallest
	// they draw at.
	pics, picRows int
}

// descBody is the body of the Description pane in box b.
func (m *detailModal) descBody(b panes.Box) string {
	in := b.Rect.Inner()
	p := m.side
	if !m.loaded || in.W <= 0 || p == nil {
		return ""
	}
	d := &p.desc
	d.tree.SetSize(in.W, in.H)
	k := descKey{text: m.issue.Body, width: in.W, look: d.look, pics: d.pics, picRows: m.pics.PictureRows(in.H)}
	if !d.made || d.built != k {
		// Images draw as the app's images setting says: none while it is off.
		if m.md != nil {
			m.md.SetPictures(m.pics.Pictures(k.picRows, nil))
		}
		set := m.icons.Set()
		if set == "" {
			set = config.IconsNerd
		}
		var rows []treeview.Node
		if strings.TrimSpace(m.issue.Body) != "" {
			rows = append(rows, desc.Rows(m.issue.Body, desc.Options{Width: in.W, Styles: desc.NewStyles(m.theme, m.icons, set), MD: m.md})...)
		}
		d.tree.SetStyles(m.theme.Tree(m.icons))
		d.tree.SetRows(rows)
		d.built, d.made = k, true
	}
	if m.nav.Focus() == paneDescription {
		d.tree.Focus()
	} else {
		d.tree.Blur()
	}
	return d.tree.View()
}

// newDesc makes the tree of the Description pane.
func newDesc(keys config.Keymap) treeview.Model {
	return treeview.New(treeview.Source{}, treeview.WithKeyMap(sideKeys(keys)))
}

package pulls

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/desc"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// ctxDescription is the context of the keys of the description.
const ctxDescription = "pull_description"

// descPane is the description of the pull request: its body as the rows of a
// tree, with the details blocks as folds. Its rows are made again only when
// the body, the width or the look changes; the first draw makes them.
type descPane struct {
	tree treeview.Model
	md   *markdown.Renderer
	// rows are the rows in the tree, which the scroll position is counted from.
	rows []treeview.Node
	// built is what the rows were made from, and made says there are rows.
	built descKey
	made  bool
	// pics counts the changes of the images that the rows draw.
	pics int
	// version counts the changes of what the tree shows that its own key
	// presses and the images make, which the drawn panes depend on.
	version int
	// links counts the answers of the autolinks that the rows draw, and ask is
	// the body drawn since the last update, to ask about its references.
	links int
	// drew is the generation of the autolinks the rows were drawn with.
	drew uint64
	ask  string
}

// descKey is what the rows of the description depend on.
type descKey struct {
	body    string
	width   int
	gen     int
	picRows int
	pics    int
	links   int
}

// position returns the line of the cursor, from 1, and the lines the tree
// shows with its closed folds hidden.
func (d *descPane) position() (at, total int) {
	var sel string
	if n, ok := d.tree.Selected(); ok {
		sel = n.ID
	}
	hide := -1
	for i := range d.rows {
		r := &d.rows[i]
		if hide >= 0 && r.Depth > hide {
			continue
		}
		hide = -1
		n := len(r.Lines)
		if r.Branch && d.tree.Folded(r.ID) {
			n, hide = 1, r.Depth
		}
		if r.ID == sel {
			at = total + 1
		}
		total += n
	}
	return at, total
}

// description is the content of the Description pane: the body in the tree,
// and the dim right-hand info, which says the body was edited and how far it
// is scrolled once it overflows.
func (m *panesModal) description(b panes.Box) panes.Content {
	in := b.Rect.Inner()
	body := strings.TrimSpace(m.detail.Body)
	if body == "" {
		m.desc.made = false
		return panes.Content{Body: m.theme.Muted.Render("No description")}
	}
	d := &m.desc
	d.sync(m, body, in.W, in.H)
	if m.nav.Focus() == paneDescription {
		d.tree.Focus()
	} else {
		d.tree.Blur()
	}
	c := panes.Content{Body: d.tree.View()}
	var info []string
	if !m.detail.BodyEditedAt.IsZero() {
		info = append(info, "edited")
	}
	if at, total := d.position(); total > in.H && in.H > 0 {
		info = append(info, fmt.Sprintf("%d%%", min(at*100/total, 100)))
	}
	if len(info) > 0 {
		c.TopRight = []string{strings.Join(info, m.icons.Separator)}
	}
	return c
}

// sync makes the rows again if what they depend on changed, and sizes the tree.
func (d *descPane) sync(m *panesModal, body string, width, height int) {
	k := descKey{body, width, m.gen, m.pics.PictureRows(m.height), d.pics, d.links}
	if !d.made {
		d.tree = treeview.New(treeview.Source{}, treeview.WithKeyMap(m.keys.desc))
	}
	d.tree.SetSize(width, height)
	if d.made && d.built == k {
		return
	}
	if d.md == nil || d.built.gen != m.gen {
		d.md = markdown.New(m.theme.Thread(m.icons).Markdown)
		d.md.SetGlyphs(m.icons.Markdown)
		d.md.SetLinkColor(m.theme.LinkColor())
	}
	d.md.SetAutolink(m.links().Func(m.repo.String()))
	d.askLinks(body)
	set := config.IconsNerd
	if m.icons.Ellipsis == "..." {
		set = config.IconsASCII
	}
	// Images draw as the app's images setting says: none while it is off.
	d.md.SetPictures(m.pics.Pictures(k.picRows, nil))
	d.tree.SetStyles(m.theme.TreeView(m.icons))
	d.rows = desc.Rows(body, desc.Options{Width: width, Styles: desc.NewStyles(m.theme, m.icons, set), MD: d.md})
	d.tree.SetRows(d.rows)
	d.built, d.made = k, true
	d.version++
}

// imagesChanged has the description draw its images again, once the images
// the app holds changed.
func (m *panesModal) imagesChanged() {
	m.desc.pics++
	m.desc.version++
}

// pressDescription takes a key of the description's own: the tree moves the
// cursor and opens and closes the details blocks, and the keys that edit the
// title and the description and quote it. It takes only a key that one of
// them binds, so that any other key leaves the drawn panes as they are.
func (m *panesModal) pressDescription(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	// Editing and quoting work on an empty description too: the first adds one,
	// the second says there is nothing to quote.
	switch {
	case keymap.Matches(msg, m.keys.editDesc):
		return m.openDescEdit(), true
	case keymap.Matches(msg, m.keys.quoteDesc):
		return m.quoteDescription(), true
	}
	d := &m.desc
	if !d.made {
		return nil, false
	}
	var bound bool
	for _, row := range m.keys.desc.FullHelp() {
		bound = bound || keymap.Matches(msg, row...)
	}
	if !bound {
		return nil, false
	}
	d.tree.Focus()
	var cmd tea.Cmd
	d.tree, cmd = d.tree.Update(msg)
	d.version++
	return cmd, true
}

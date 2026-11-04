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
}

// descKey is what the rows of the description depend on.
type descKey struct {
	text  string
	width int
	look  int
}

// metaLines are the lines above the body: who opened the issue and when,
// whom it is assigned to, and how many comments it has.
func (m *detailModal) metaLines() []string {
	it := m.issue
	now := m.now()
	meta := login(it.Author) + " opened " + m.dates.Prose(it.CreatedAt, now)
	if it.UpdatedAt.After(it.CreatedAt) {
		meta += ", updated " + m.dates.Prose(it.UpdatedAt, now)
	}
	lines := []string{m.theme.Subtle.Render(meta)}
	if len(it.Assignees) > 0 {
		names := make([]string, len(it.Assignees))
		for i, a := range it.Assignees {
			names[i] = login(a)
		}
		lines = append(lines, m.theme.Subtle.Render("assigned to "+strings.Join(names, ", ")))
	}
	return append(lines, m.theme.Subtle.Render(plural(it.Comments, "comment")), "")
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
	k := descKey{text: strings.Join(m.metaLines(), "\n") + "\n" + m.issue.Body, width: in.W, look: d.look}
	if !d.made || d.built != k {
		set := config.IconsNerd
		if m.icons.Ellipsis == "..." {
			set = config.IconsASCII
		}
		var rows []treeview.Node
		for i, l := range m.metaLines() {
			rows = append(rows, treeview.Node{ID: "~meta" + string(rune('a'+i)), Lines: []string{l}, Skip: true})
		}
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

package dev

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// NewKeyMap returns the keys of the pane, read from its context in keys. Enter
// is the select key of the global context, never bound here. The pane marks
// nothing, so the tree's mark key stays unbound.
func NewKeyMap(keys config.Keymap) foldtree.KeyMap {
	look := ui.Lookup(keys, Context)
	return foldtree.NewKeyMap(keymap.Func(func(action string) []string {
		if action == "mark" {
			return nil
		}
		return look.Of(action)
	}))
}

// Model is the pane: a tree of what a pull request links, or a line that says
// it is loading or why it failed.
type Model struct {
	tree   foldtree.Model
	items  map[string]Item
	opts   Options
	links  core.PullLinks
	loaded bool
	err    error
	// rows is the number of rows Build made, with every fold open.
	rows int
	// version counts the changes of what View draws.
	version int
	width   int
	height  int
}

// New returns a pane that has read nothing.
func New(o Options) Model {
	m := Model{opts: o, items: map[string]Item{}}
	m.tree = foldtree.New(foldtree.WithStyles(o.Styles.Tree))
	return m
}

// Version changes whenever what the pane draws does, for a parent that keeps
// the drawing.
func (m Model) Version() int { return m.version }

// SetStyles sets how the rows are drawn, and builds them again.
func (m *Model) SetStyles(s Styles) {
	m.opts.Styles = s
	m.tree.SetStyles(s.Tree)
	m.rebuild()
}

// SetRepo sets the repository of the pull request.
func (m *Model) SetRepo(r core.RepoRef) {
	m.opts.Repo = r
	m.rebuild()
}

// SetKeyMap sets the keys of the tree.
func (m *Model) SetKeyMap(k foldtree.KeyMap) { m.tree.SetKeyMap(k) }

// SetSize sets the size of the pane in cells.
func (m *Model) SetSize(width, height int) {
	width, height = max(width, 0), max(height, 0)
	if width == m.width && height == m.height {
		return
	}
	m.width, m.height = width, height
	m.tree.SetSize(width, height)
	m.version++
}

// Focus makes the pane take keys, and Blur makes it ignore them.
func (m *Model) Focus() { m.tree.Focus() }

// Blur makes the pane ignore keys.
func (m *Model) Blur() { m.tree.Blur() }

// SetLinks shows links, and forgets an error.
func (m *Model) SetLinks(links core.PullLinks) {
	m.links, m.loaded, m.err = links, true, nil
	m.rebuild()
}

// SetError says that the links couldn't be read. Links read before stay.
func (m *Model) SetError(err error) {
	m.err = err
	m.version++
}

// Loaded reports whether the pane has links to show.
func (m Model) Loaded() bool { return m.loaded }

// Rows is the number of rows the pane has with every fold open, which is how
// they start, or 0 before it has links.
func (m Model) Rows() int { return m.rows }

func (m *Model) rebuild() {
	if m.loaded {
		r := Build(m.links, m.opts)
		m.items = r.Items
		m.rows = len(r.Rows)
		m.tree.SetRows(r.Rows)
	}
	m.version++
}

// Selected returns what the row under the cursor stands for.
func (m Model) Selected() (Item, bool) {
	id, ok := m.tree.Selected()
	if !ok {
		return Item{}, false
	}
	it, ok := m.items[id]
	return it, ok
}

// Item returns what the row with the identity id stands for.
func (m Model) Item(id string) (Item, bool) {
	it, ok := m.items[id]
	return it, ok
}

// Closes is the number of issues the pull request closes, as GitHub counts
// them, which may be more than the rows show.
func (m Model) Closes() int { return m.links.ClosesTotal }

// Update moves the cursor and the folds on a key, when the pane has the
// focus. It returns the message of an enter that the tree leaves to the
// caller.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyPressMsg); !ok || !m.loaded {
		return m, nil
	}
	var cmd tea.Cmd
	m.tree, cmd = m.tree.Update(msg)
	m.version++
	return m, cmd
}

// ToggleFold folds the group that id names, or opens it.
func (m *Model) ToggleFold(id string) {
	m.tree.SetFolded(id, !m.tree.Folded(id))
	m.version++
}

// ID returns the identity that the messages of the tree carry.
func (m Model) ID() int { return m.tree.ID() }

// View draws the pane in exactly the width and height it has.
func (m Model) View() string {
	st := m.opts.Styles
	switch {
	case m.loaded:
		return m.tree.View()
	case m.err != nil:
		return fit(st.warn.Render("can't read what this pull request links: "+ui.OneLine(m.err.Error())), m.height)
	}
	return fit(st.dim.Render("loading"+st.Glyphs.Ellipsis), m.height)
}

// fit returns line at the top of a block of height lines.
func fit(line string, height int) string {
	if height <= 0 {
		return ""
	}
	out := make([]string, height)
	out[0] = line
	return strings.Join(out, "\n")
}

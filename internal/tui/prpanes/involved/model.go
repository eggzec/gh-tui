package involved

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// Model is the tree of the people. It holds no keys of its own: the key map of
// its tree does, and enter on a person comes back as a [FocusPersonMsg].
type Model struct {
	tree   foldtree.Model
	folds  map[string]bool
	people map[string]Person
	// commenters is whether the group of the commenters started closed, so
	// that a conversation that arrives later can still decide it, unless the
	// user folded it already.
	commenters bool
}

// New returns a model with no rows, made with the options of a foldtree.
func New(opts ...foldtree.Option) Model {
	return Model{tree: foldtree.New(opts...)}
}

// SetData sets what the pane shows. The cursor and the folds stay with the IDs
// of the rows. The group of the commenters follows the default until the user
// folds it.
func (m *Model) SetData(d Data, o Options) {
	rows, who := build(d, o)
	m.folds = map[string]bool{}
	for _, r := range rows {
		if r.Fold {
			m.folds[r.ID] = true
		}
	}
	m.people = who
	was := m.commenters
	m.commenters = DefaultFolds(d)[idCommenters]
	untouched := m.tree.Folded(idCommenters) == was
	m.tree.SetRows(rows)
	if untouched && was != m.commenters {
		m.tree.SetFolded(idCommenters, m.commenters)
	}
}

// Update handles keys while the tree is focused, and turns the enter that its
// tree reports into the message of the row.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	enter, ok := msg.(foldtree.EnterMsg)
	if !ok {
		var cmd tea.Cmd
		m.tree, cmd = m.tree.Update(msg)
		return m, cmd
	}
	if enter.Tree != m.tree.ID() {
		return m, nil
	}
	if p, ok := m.people[enter.ID]; ok {
		focus := FocusPersonMsg{Login: p.Login, Team: p.Team, Name: p.Name}
		return m, func() tea.Msg { return focus }
	}
	if m.folds[enter.ID] {
		m.tree.SetFolded(enter.ID, !m.tree.Folded(enter.ID))
	}
	return m, nil
}

// Selected returns the person or team under the cursor, and false on a row
// that stands for none.
func (m Model) Selected() (Person, bool) {
	id, ok := m.tree.Selected()
	if !ok {
		return Person{}, false
	}
	p, ok := m.people[id]
	return p, ok
}

// SelectedID returns the ID of the row under the cursor, as [Rows] names it.
func (m Model) SelectedID() (string, bool) { return m.tree.Selected() }

// View returns the rows drawn in the size of the tree.
func (m Model) View() string { return m.tree.View() }

// ID returns the identity that the messages of the tree carry.
func (m Model) ID() int { return m.tree.ID() }

// SetSize sets the width and height of the tree in cells.
func (m *Model) SetSize(width, height int) { m.tree.SetSize(width, height) }

// SetKeyMap sets the key bindings of the tree.
func (m *Model) SetKeyMap(k foldtree.KeyMap) { m.tree.SetKeyMap(k) }

// KeyMap returns the key bindings of the tree.
func (m Model) KeyMap() foldtree.KeyMap { return m.tree.KeyMap() }

// SetStyles sets the styles of the tree.
func (m *Model) SetStyles(s foldtree.Styles) { m.tree.SetStyles(s) }

// Focus makes the tree take keys, and Blur makes it ignore them.
func (m *Model) Focus() { m.tree.Focus() }

// Blur makes the tree ignore keys.
func (m *Model) Blur() { m.tree.Blur() }

// Folded reports whether the group with the ID id is closed.
func (m Model) Folded(id string) bool { return m.tree.Folded(id) }

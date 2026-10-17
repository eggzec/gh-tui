package merge

import (
	"maps"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// Model is the tree of the blockers. It holds no keys of its own: the key map
// of its tree does, and enter on a row comes back as the intent message that
// the row asks for, or as the message of a job or check of the checks.
type Model struct {
	tree    foldtree.Model
	rows    []foldtree.Row
	targets checktree.Targets
}

// New returns a model with no rows, made with the options of a foldtree.
func New(opts ...foldtree.Option) Model {
	return Model{tree: foldtree.New(opts...)}
}

// SetRows sets the rows and what enter on them asks for, as [Rows] returns
// them. The cursor and the folds stay with the IDs of the rows.
func (m *Model) SetRows(rows []foldtree.Row, targets checktree.Targets) {
	m.rows = slices.Clone(rows)
	m.targets = maps.Clone(targets)
	m.tree.SetRows(rows)
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
	if target, ok := m.targets[enter.ID]; ok && target != nil {
		return m, func() tea.Msg { return target }
	}
	for i := range m.rows {
		if m.rows[i].ID == enter.ID && m.rows[i].Fold {
			m.tree.SetFolded(enter.ID, !m.tree.Folded(enter.ID))
			break
		}
	}
	return m, nil
}

// View returns the rows drawn in the size of the tree.
func (m Model) View() string { return m.tree.View() }

// ID returns the identity that the messages of the tree carry.
func (m Model) ID() int { return m.tree.ID() }

// SetSize sets the width and height of the tree in cells.
func (m *Model) SetSize(width, height int) { m.tree.SetSize(width, height) }

// SetKeyMap sets the key bindings of the tree.
func (m *Model) SetKeyMap(k foldtree.KeyMap) { m.tree.SetKeyMap(k) }

// SetStyles sets the styles of the tree.
func (m *Model) SetStyles(s foldtree.Styles) { m.tree.SetStyles(s) }

// Focus makes the tree take keys, and Blur makes it ignore them.
func (m *Model) Focus() { m.tree.Focus() }

// Blur makes the tree ignore keys.
func (m *Model) Blur() { m.tree.Blur() }

// Selected returns the ID of the row under the cursor.
func (m Model) Selected() (id string, ok bool) { return m.tree.Selected() }

// Folded reports whether the fold with the ID id is closed.
func (m Model) Folded(id string) bool { return m.tree.Folded(id) }

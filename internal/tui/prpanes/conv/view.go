package conv

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// cacheKey says what the rows of a view were built from. Rows are built
// again only when it changes, never when the cursor or a fold moves.
type cacheKey struct {
	width, data, opts int
}

// view is a tree of the conversation, with the state that is its own: cursor,
// scroll and folds.
type view struct {
	tree foldtree.Model
	// folds holds the folds the user set, by row ID. A tree forgets the fold of
	// a row that leaves it, as a filter or a reload may take it away, and this
	// remembers it for when it comes back.
	folds map[string]bool
	// rows are the rows as the tree has them, and nodes the entries they
	// were built from, by ID.
	rows  []foldtree.Row
	nodes map[string]*node
	key   cacheKey
	done  bool
}

// view returns the view with key, made when it is new.
func (m *Model) view(key string) *view {
	if v, ok := m.views[key]; ok {
		return v
	}
	t := foldtree.New()
	t.SetKeyMap(m.keys)
	if m.hasTreeStyles {
		t.SetStyles(m.treeStyles)
	}
	t.SetSize(m.width, m.height)
	if m.focused {
		t.Focus()
	}
	v := &view{tree: t, folds: map[string]bool{}}
	m.views[key] = v
	return v
}

// sync builds the rows of the view that shows if what they are built from
// changed.
func (m *Model) sync() {
	if !m.ready() || m.width < minWidth {
		return
	}
	v := m.views[m.cur]
	k := cacheKey{m.width, m.version, m.optsVer}
	if v.done && v.key == k {
		return
	}
	m.rebuild(v)
	v.key, v.done = k, true
}

// rebuild builds the rows of v and gives them to its tree, which keeps the
// cursor by ID.
func (m *Model) rebuild(v *view) {
	for _, r := range v.rows {
		if !r.Fold {
			continue
		}
		if c := v.tree.Folded(r.ID); c != r.Closed {
			v.folds[r.ID] = c
		}
	}
	o := m.opts
	o.Width, o.Viewer = m.width, m.viewer
	rows, nodes := build(m.dataOf(), o, true)
	for i, r := range rows {
		if c, ok := v.folds[r.ID]; ok && r.Fold {
			rows[i].Closed = c
		}
	}
	rows = m.withMore(rows, o)
	v.rows, v.nodes = rows, nodes
	v.tree.SetRows(rows)
	if m.built != nil {
		m.built()
	}
}

// dataOf returns the data the rows are built from.
func (m Model) dataOf() Data { return Data{Items: m.items, Threads: m.threads} }

// withMore adds the quiet last row that says how much of the conversation
// GitHub holds and was not read, when it is more than the reads reached.
func (m Model) withMore(rows []foldtree.Row, o Options) []foldtree.Row {
	tl, th := m.feeds[timeline], m.feeds[threads]
	var text string
	switch {
	case tl.done && tl.err == nil && tl.truncated:
		text = "more not shown"
	case th.done && th.err == nil && m.total > len(m.threads):
		text = strconv.Itoa(m.total-len(m.threads)) + " more not shown"
	default:
		return rows
	}
	st := &o.Styles
	return append(rows,
		foldtree.Row{ID: "b:more", Lines: []string{""}, Skip: true, Kind: KindBlank},
		foldtree.Row{ID: "more", Lines: []string{st.dim.Render(text)}, Skip: true},
	)
}

// enter returns the command that tells the host which row was entered.
func (v *view) enter(id string) tea.Cmd {
	n := v.nodes[id]
	if n == nil {
		return nil
	}
	msg := EnterMsg{ID: id, Kind: n.kind, Item: n.item, Thread: n.thread, Comment: n.comment}
	return func() tea.Msg { return msg }
}

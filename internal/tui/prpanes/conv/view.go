package conv

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// cacheKey says what the rows of a view were built from. Rows are built
// again only when it changes, never when the cursor or a fold moves.
type cacheKey struct {
	width, data, opts, gen, shape, checks int
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
	// targets is what enter asks for on the rows of the checks, by row ID.
	targets checktree.Targets
	key     cacheKey
	done    bool
	// narrow says what a narrowed view keeps, and gen counts the times it was
	// set, so that rows are built again for a new one.
	narrow   Narrow
	narrowed bool
	gen      int
	// base is the text of the tree when it has no rows, as it came, and
	// nothing says that the filter left none, which the text says instead.
	base    string
	nothing bool
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
	v := &view{tree: t, folds: map[string]bool{}, base: t.Styles().EmptyText}
	m.views[key] = v
	return v
}

// SetView shows the view with key, made when it is new. A Narrow makes it a
// preview of the part of the conversation it keeps; the view keeps its own
// cursor, scroll and folds. The empty key is the full conversation, which a
// Narrow never changes. A call with a Narrow builds the rows again, so call it
// when the narrowing changes, not on every frame.
func (m *Model) SetView(key string, n Narrow) {
	v := m.view(key)
	m.cur = key
	if key != "" && (n.Item != nil || n.Thread != nil) {
		v.narrow, v.narrowed = n, true
		v.gen++
	}
	m.sync()
}

// Peek draws the view with key, narrowed by n, as a preview width cells wide
// and as tall as its rows, and leaves the view that shows as it was, at its own
// size. The view is made when it is new and keeps its own folds. ok is false
// when the conversation isn't read yet, or the view has no rows. Like SetView
// with a Narrow, it builds the rows again, so call it when the narrowing or the
// width changed.
func (m *Model) Peek(key string, n Narrow, width int) (view string, ok bool) {
	if key == "" || !m.ready() || width < minWidth {
		return "", false
	}
	v := m.view(key)
	v.narrow, v.narrowed = n, true
	v.gen++
	cur, w, h := m.cur, m.width, m.height
	m.cur, m.width, m.height = key, width, 1
	v.tree.SetSize(width, 1)
	m.sync()
	m.cur, m.width, m.height = cur, w, h
	lines := 0
	for _, r := range v.rows {
		lines += max(len(r.Lines), 1)
	}
	if lines == 0 {
		return "", false
	}
	v.tree.SetSize(width, lines)
	return strings.TrimRight(v.tree.View(), " \n"), true
}

// sync builds the rows of the view that shows if what they are built from
// changed.
func (m *Model) sync() {
	if !m.ready() || m.width < minWidth {
		return
	}
	v := m.views[m.cur]
	k := cacheKey{m.width, m.version, m.optsVer, v.gen, m.shape, m.checksVer}
	if v.narrowed {
		// A preview keeps every group, and has no commits.
		k.shape, k.checks = 0, 0
	}
	if v.done && v.key == k {
		return
	}
	m.rebuild(v, v.done && v.key.shape != k.shape)
	v.key, v.done = k, true
}

// rebuild builds the rows of v and gives them to its tree, which keeps the
// cursor by ID. With reshaped, the filter changed, and the cursor goes where
// it belongs when its row is gone.
func (m *Model) rebuild(v *view, reshaped bool) {
	cur, hadCur := cursorOf(v)
	for _, r := range v.rows {
		if !r.Fold {
			continue
		}
		if c := v.tree.Folded(r.ID); c != r.Closed {
			v.folds[r.ID] = c
		}
	}
	o := m.opts
	o.Width, o.Viewer, o.hide, o.order, o.checks = m.width, m.viewer, m.hide(), m.order, m.checks
	targets := checktree.Targets{}
	if m.src.Checks != nil {
		o.kids = func(prefix, sha string, depth int) []foldtree.Row {
			rows, t := m.loader.Rows(sha, m.checkOptions(prefix, depth), o.Styles.checks)
			maps.Copy(targets, t)
			return checktree.FoldRows(rows)
		}
	}
	d := m.dataOf()
	if v.narrowed {
		d, o.preview = v.narrow.filter(d), true
	}
	rows, nodes := build(d, o, true)
	for i, r := range rows {
		if c, ok := v.folds[r.ID]; ok && r.Fold {
			rows[i].Closed = c
		}
	}
	if !v.narrowed {
		_, total := m.Shown()
		v.nothing = len(rows) == 0 && total > 0
		rows = m.withMore(rows, o)
	}
	v.rows, v.nodes, v.targets = rows, nodes, targets
	v.tree.SetRows(rows)
	m.applyEmpty(v)
	if reshaped && hadCur {
		m.keepCursor(v, cur)
	}
	// After the cursor moved, since it opens the folds above its row.
	v.syncForms()
	if m.built != nil {
		m.built()
	}
}

// syncForms gives each push the line of the form its fold shows: centred
// closed, at the left open. The rows are copied before one changes, since the
// tree and the copies of the model share them, and the tree gets them again
// only when a line changed.
func (v *view) syncForms() {
	var rows []foldtree.Row
	for i, r := range v.rows {
		if r.Kind != KindPush {
			continue
		}
		n := v.nodes[r.ID]
		if n == nil {
			continue
		}
		form := 1
		if v.tree.Folded(r.ID) {
			form = 0
		}
		if r.Lines[0] == n.forms[form] {
			continue
		}
		if rows == nil {
			rows = slices.Clone(v.rows)
		}
		rows[i].Lines = []string{n.forms[form]}
	}
	if rows != nil {
		v.rows = rows
		v.tree.SetRows(rows)
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

// enter returns the command that tells the host which row was entered: what a
// row of the checks asks for, else the row itself.
func (v *view) enter(id string) tea.Cmd {
	if t, ok := v.targets[id]; ok {
		return func() tea.Msg { return t }
	}
	n := v.nodes[id]
	if n == nil {
		return nil
	}
	msg := EnterMsg{ID: id, Kind: n.kind, Item: n.item, Thread: n.thread, Comment: n.comment}
	if n.kind == KindCommit || n.kind == KindCI {
		msg.SHA = n.item.Commit.OID
	}
	return func() tea.Msg { return msg }
}

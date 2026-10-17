package conv

import (
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// noMatch is the text of a conversation whose filter leaves nothing.
const noMatch = "nothing matches this filter"

// SetShown sets which groups the conversation shows, from the choices the user
// made, and builds the rows again. The cursor stays on its row when that is
// still there, and else goes to the row it belonged to, or the one before it.
// The folds the user set come back with their rows.
func (m *Model) SetShown(on [NumGroups]bool) {
	if m.shown == on {
		return
	}
	m.shown = on
	m.shape++
	m.sync()
}

// SetFilterHint sets the key that changes the filter, as the host binds it, for
// the text of a conversation that the filter leaves empty. "" says that no key
// is bound, and leaves the hint out.
func (m *Model) SetFilterHint(key string) {
	m.hint = key
	for _, v := range m.views {
		m.applyEmpty(v)
	}
}

// hide returns the groups that are left out.
func (m Model) hide() (h [NumGroups]bool) {
	for g, on := range m.shown {
		h[g] = !on
	}
	return h
}

// applyEmpty gives the tree of v the text that says why it is empty: the
// filter's when it hid everything, else the text it was given.
func (m *Model) applyEmpty(v *view) {
	want := v.base
	if m.hasTreeStyles {
		want = m.treeStyles.EmptyText
	}
	if v.nothing {
		want = noMatch
		if m.hint != "" {
			want += " (" + m.hint + " changes it)"
		}
	}
	s := v.tree.Styles()
	if s.EmptyText == want {
		return
	}
	s.EmptyText = want
	v.tree.SetStyles(s)
}

// cursor is where the cursor of a view was before its rows were built again:
// the row, the rows it was under, nearest first, and the nodes of all rows.
type cursor struct {
	id    string
	above []string
	nodes map[string]*node
}

// cursorOf returns where the cursor of v is, and false when it has no row.
func cursorOf(v *view) (cursor, bool) {
	id, ok := v.tree.Selected()
	if !ok {
		return cursor{}, false
	}
	c := cursor{id: id, nodes: v.nodes}
	i := slices.IndexFunc(v.rows, func(r foldtree.Row) bool { return r.ID == id })
	if i < 0 {
		return c, true
	}
	depth := v.rows[i].Depth
	for k := i - 1; k >= 0 && depth > 0; k-- {
		if r := v.rows[k]; r.Depth < depth {
			depth = r.Depth
			c.above = append(c.above, r.ID)
		}
	}
	return c, true
}

// keepCursor puts the cursor of v, whose rows were just built again, where it
// belongs: on its row, if that is still there, else on the row it was a part
// of, and else on the row that came before it in the order of the timeline, or
// the first.
func (m *Model) keepCursor(v *view, c cursor) {
	target := m.landing(v, c)
	if target == "" && len(v.rows) > 0 {
		target = v.rows[0].ID
	}
	if target != "" {
		v.tree.Reveal(target)
	}
}

// landing returns the ID of the row that takes the cursor, or "".
func (m *Model) landing(v *view, c cursor) string {
	if _, ok := v.nodes[c.id]; ok {
		return c.id
	}
	// A comment is in a run or stands alone, as the filter leaves it, and a
	// run goes with its first comment.
	// A system line goes with the line that holds its event now, which may
	// be an earlier line that it merged into.
	if n := c.nodes[c.id]; n != nil && (n.item != nil || n.kind == KindRun) {
		if id := rowOf(v.nodes, anchorOf(n)); id != "" {
			return id
		}
	}
	for _, id := range c.above {
		if _, ok := v.nodes[id]; ok {
			return id
		}
	}

	// The row before it among all that the data has, in GitHub's order.
	var anchors []string
	for _, n := range tree(m.dataOf(), Options{}) {
		switch {
		case n.kind == KindRun:
			for _, k := range n.children {
				anchors = append(anchors, k.item.ID)
			}
		case n.kind == KindEvent:
			for _, it := range n.events {
				anchors = append(anchors, eventAnchor(it))
			}
		case n.item != nil:
			anchors = append(anchors, n.item.ID)
		}
	}
	at := len(anchors)
	top := c.nodes[c.id]
	if len(c.above) > 0 {
		top = c.nodes[c.above[len(c.above)-1]]
	}
	if top != nil {
		if i := slices.Index(anchors, anchorOf(top)); i >= 0 {
			at = i
		}
	}
	for i := at - 1; i >= 0; i-- {
		if id := rowOf(v.nodes, anchors[i]); id != "" {
			return id
		}
	}
	return ""
}

// eventAnchor names an event for rowOf. Events have no ID from GitHub, and the
// ID of a line changes when lines merge, so the anchor is the event's own key.
func eventAnchor(it *core.TimelineItem) string { return "e:" + eventKey(it) }

// rowOf returns the ID of the row of the comment or review with the ID of
// an item, alone or in a run, or "" if it has no row. For an event anchor it
// is the system line that holds the event, merged or not.
func rowOf(nodes map[string]*node, item string) string {
	if strings.HasPrefix(item, "e:") {
		for id, n := range nodes {
			if n.kind == KindEvent && slices.ContainsFunc(n.events, func(it *core.TimelineItem) bool { return eventAnchor(it) == item }) {
				return id
			}
		}
		return ""
	}
	for _, id := range []string{"c:" + item, "rc:" + item, "r:" + item} {
		if _, ok := nodes[id]; ok {
			return id
		}
	}
	return ""
}

// anchorOf returns the ID of the item that a top-level node stands for: its
// own, or the first of a run's, or the review of a thread.
func anchorOf(n *node) string {
	switch {
	case n.kind == KindEvent:
		return eventAnchor(n.item)
	case n.item != nil:
		return n.item.ID
	case n.kind == KindRun && len(n.children) > 0:
		return anchorOf(n.children[0])
	case n.thread != nil:
		return n.thread.ReviewID
	}
	return ""
}

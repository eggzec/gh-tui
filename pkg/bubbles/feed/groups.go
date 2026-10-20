package feed

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Group is one level of grouping: it tells which group an item belongs to.
type Group[T any] struct {
	// Key returns the key of the group of an item. Groups are ordered by key,
	// and the empty key makes the group that comes last, called "none".
	Key func(T) string
	// Label returns what a group's header reads for its key. Without it the
	// header reads the key.
	Label func(key string) string
	// Desc orders the groups of this level from Z to A.
	Desc bool
}

// line is one row of a grouped feed: a group header, an item, or the row after
// the last one that stands for the items not loaded yet.
type line struct {
	head  bool
	more  bool
	depth int
	// path identifies a header among the others, from the keys of the groups
	// that lead to it.
	path  string
	label string
	// item is the index of an item line's item among all items.
	item int
	// count is the number of loaded items in a header's group, and open
	// whether its lines show.
	count int
	open  bool
	// before is the number of loaded items that come before the line in the
	// order of the groups, whether they show or not.
	before int
}

// cursorRef remembers what the cursor is on, to find it in new lines.
type cursorRef struct {
	valid bool
	head  bool
	more  bool
	path  string
	item  int
	key   string
	byKey bool
}

// SetGroups groups the rows by the given keys, the first group at the top
// level and each next inside it, or shows the plain list again for none. Every
// group starts open, and the cursor stays on its item. While the groups are
// set the feed counts lines, headers and items, rather than items, and a quick
// filter narrows the items before they are grouped, so the headers count
// only the rows it keeps. A feed whose items are not all loaded ends in a row
// that stands for the rest, which is not an item: see [Model.Activate] and
// [Model.SetTotal].
func (m *Model[T]) SetGroups(groups []Group[T]) tea.Cmd {
	ref := m.cursor()
	m.groups = m.groups[:0:0]
	for _, g := range groups {
		if g.Key != nil {
			m.groups = append(m.groups, g)
		}
	}
	m.closed = nil
	m.anchored, m.seeking, m.fill = false, false, true
	if !m.Grouped() {
		m.sel = m.flatSel()
		m.lines, m.heads, m.order, m.nitems = nil, nil, nil, 0
		if m.filter != "" {
			m.rebuildRows()
		} else {
			m.refind()
		}
	} else {
		m.regroup(ref)
	}
	m.enableKeys()
	return m.sync()
}

// regroup builds the lines again over the items the quick filter keeps, and
// the find over those, and puts the cursor back on ref. It reports whether
// ref was found.
func (m *Model[T]) regroup(ref cursorRef) bool {
	m.filterRows()
	m.build()
	found := m.restore(ref)
	m.refind()
	return found
}

// Grouped reports whether the rows are grouped.
func (m Model[T]) Grouped() bool {
	return len(m.groups) > 0
}

// Expand opens the group of the header under the cursor.
func (m *Model[T]) Expand() tea.Cmd {
	return m.setOpen(true)
}

// Collapse closes the group of the header under the cursor. On an item it
// closes the group the item is in, and moves the cursor to its header.
func (m *Model[T]) Collapse() tea.Cmd {
	return m.setOpen(false)
}

// ExpandAll opens every group.
func (m *Model[T]) ExpandAll() tea.Cmd {
	if !m.Grouped() {
		return nil
	}
	ref := m.cursor()
	m.closed, m.fill = nil, true
	m.build()
	m.restore(ref)
	return m.sync()
}

// CollapseAll closes every group, and moves the cursor to the header at the
// top of the one it was in.
func (m *Model[T]) CollapseAll() tea.Cmd {
	if !m.Grouped() {
		return nil
	}
	ref := m.headRef(0)
	// Closing adds no line, so no chunk is fetched just to fill the window.
	m.fill = false
	m.closed = make(map[string]struct{}, len(m.heads))
	for _, p := range m.heads {
		m.closed[p] = struct{}{}
	}
	m.build()
	m.restore(ref)
	return m.sync()
}

// setOpen opens or closes the group of the header under the cursor.
func (m *Model[T]) setOpen(open bool) tea.Cmd {
	if !m.Grouped() || m.sel < 0 || m.sel >= len(m.lines) {
		return nil
	}
	l := m.lines[m.sel]
	if !l.head && open {
		return nil
	}
	ref := m.cursor()
	if !l.head {
		ref = m.headRef(l.depth - 1)
	}
	if ref.path == "" {
		return nil
	}
	closed := maps.Clone(m.closed)
	if closed == nil {
		closed = map[string]struct{}{}
	}
	if open {
		delete(closed, ref.path)
	} else {
		closed[ref.path] = struct{}{}
	}
	m.closed, m.fill = closed, open
	m.build()
	m.restore(ref)
	return m.sync()
}

// headRef returns a reference to the header at depth that the cursor is on
// or under, or an empty one.
func (m Model[T]) headRef(depth int) cursorRef {
	for i := min(m.sel, len(m.lines)-1); i >= 0; i-- {
		if l := m.lines[i]; l.head && l.depth == depth {
			return cursorRef{valid: true, head: true, path: l.path}
		}
	}
	return cursorRef{}
}

// right is the key that goes into a group: it opens a closed header, and on
// an open one moves to its first line. It does nothing on an item.
func (m *Model[T]) right() tea.Cmd {
	if !m.onHeader() {
		return nil
	}
	l := m.lines[m.sel]
	if !l.open {
		return m.setOpen(true)
	}
	// An open header has items, and its first line is the next one.
	if m.sel+1 < len(m.lines) && m.lines[m.sel+1].depth > l.depth {
		m.sel++
		return m.sync()
	}
	return nil
}

// left is the key that goes out of a group: it closes an open header, and
// from an item or a closed header moves to the header of the group around it.
func (m *Model[T]) left() tea.Cmd {
	if !m.Grouped() || m.sel < 0 || m.sel >= len(m.lines) {
		return nil
	}
	l := m.lines[m.sel]
	if l.more {
		return nil
	}
	if l.head && l.open {
		return m.setOpen(false)
	}
	for i := m.sel - 1; i >= 0; i-- {
		if p := m.lines[i]; p.head && p.depth < l.depth {
			m.sel = i
			return m.sync()
		}
	}
	return nil
}

// toggleAll opens every group if any is closed, and closes them all otherwise.
func (m *Model[T]) toggleAll() tea.Cmd {
	for _, p := range m.heads {
		if _, closed := m.closed[p]; closed {
			return m.ExpandAll()
		}
	}
	return m.CollapseAll()
}

// onHeader reports whether the cursor is on a group header.
func (m Model[T]) onHeader() bool {
	return m.Grouped() && m.sel >= 0 && m.sel < len(m.lines) && m.lines[m.sel].head
}

// onMore reports whether the cursor is on the row of the items not loaded.
func (m Model[T]) onMore() bool {
	return m.Grouped() && m.sel >= 0 && m.sel < len(m.lines) && m.lines[m.sel].more
}

// Activate is what enter does on the row of the items not loaded: it fetches
// the next chunk, if there is one to fetch. It reports whether the cursor was
// on that row, so the parent knows the key was taken, and does nothing for
// any other row.
func (m *Model[T]) Activate() (tea.Cmd, bool) {
	if !m.onMore() {
		return nil, false
	}
	return m.fetchMore(), true
}

// SetTotal tells how many items the source holds in all, so that the row of
// the items not loaded can say how many are left. Zero means it is not known.
func (m *Model[T]) SetTotal(n int) {
	m.known = n
}

// moreText reads the row of the items not loaded.
func (m Model[T]) moreText() string {
	if left := m.known - m.total; m.known > 0 && left > 0 {
		return strconv.Itoa(left) + " more not loaded"
	}
	return "more not loaded"
}

// members returns the items the groups cover, in the order of the feed: those
// the quick filter keeps, or every loaded one without it.
func (m Model[T]) members() []int {
	if m.filter != "" {
		return m.rows
	}
	var items []int
	for i := range m.total {
		if _, ok := m.item(i); ok {
			items = append(items, i)
		}
	}
	return items
}

// build makes the lines of the groups over the loaded items.
func (m *Model[T]) build() {
	m.lines, m.heads, m.order = nil, nil, nil
	items := m.members()
	m.nitems = len(items)
	n := 0
	m.group(items, 0, "", true, &n)
	if !m.done && len(m.lines) > 0 {
		m.lines = append(m.lines, line{more: true, before: n})
	}
}

// group adds the lines of items, grouped by the groups from level on. A group
// that is closed, or inside one, adds no lines but still counts its items.
func (m *Model[T]) group(items []int, level int, parent string, visible bool, n *int) {
	if level == len(m.groups) {
		for _, i := range items {
			m.order = append(m.order, i)
			if visible {
				m.lines = append(m.lines, line{depth: level, item: i, before: *n})
			}
			*n++
		}
		return
	}
	g := m.groups[level]
	var keys []string
	buckets := map[string][]int{}
	for _, i := range items {
		it, _ := m.item(i)
		k := g.Key(it)
		if _, ok := buckets[k]; !ok {
			keys = append(keys, k)
		}
		buckets[k] = append(buckets[k], i)
	}
	slices.SortFunc(keys, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "":
			return 1
		case b == "":
			return -1
		case g.Desc:
			return strings.Compare(b, a)
		}
		return strings.Compare(a, b)
	})
	for _, k := range keys {
		path := parent + "\x1f" + k
		_, closed := m.closed[path]
		label := k
		switch {
		case k == "":
			label = "none"
		case g.Label != nil:
			label = g.Label(k)
		}
		m.heads = append(m.heads, path)
		if visible {
			m.lines = append(m.lines, line{
				head: true, depth: level, path: path, label: label,
				count: len(buckets[k]), open: !closed, before: *n,
			})
		}
		m.group(buckets[k], level+1, path, visible && !closed, n)
	}
}

// itemRef returns a reference to item i.
func (m Model[T]) itemRef(i int) cursorRef {
	ref := cursorRef{valid: true, item: i}
	if it, ok := m.item(i); ok && m.key != nil {
		ref.key, ref.byKey = m.key(it), true
	}
	return ref
}

// cursor returns what the cursor is on.
func (m Model[T]) cursor() cursorRef {
	if !m.Grouped() {
		if g := m.at(m.sel); g >= 0 {
			return m.itemRef(g)
		}
		return cursorRef{}
	}
	if m.sel < 0 || m.sel >= len(m.lines) {
		return cursorRef{}
	}
	switch l := m.lines[m.sel]; {
	case l.head:
		return cursorRef{valid: true, head: true, path: l.path}
	case l.more:
		return cursorRef{valid: true, more: true}
	}
	return m.itemRef(m.lines[m.sel].item)
}

// restore puts the cursor on ref in the grouped lines, if it is there, and
// leaves it where it is otherwise. It reports whether it found it.
func (m *Model[T]) restore(ref cursorRef) bool {
	if !ref.valid {
		return false
	}
	if ref.more {
		// The row stays at the end of the lines while items are missing. Once
		// the feed is done it is gone, and the last line is the nearest one.
		m.sel = max(len(m.lines)-1, 0)
		return true
	}
	for i, l := range m.lines {
		if ref.head {
			if l.head && l.path == ref.path {
				m.sel = i
				return true
			}
			continue
		}
		if l.head || l.more {
			continue
		}
		if ref.byKey {
			if it, ok := m.item(l.item); ok && m.key(it) == ref.key {
				m.sel = i
				return true
			}
		} else if l.item == ref.item {
			m.sel = i
			return true
		}
	}
	return false
}

// flatSel returns the index of the item at or after the cursor's line, for
// when the groups go and the cursor is on an item again.
func (m Model[T]) flatSel() int {
	for i := max(m.sel, 0); i < len(m.lines); i++ {
		if l := m.lines[i]; l.more {
			return max(m.shown()-1, 0)
		} else if !l.head {
			return m.posOf(l.item)
		}
	}
	return 0
}

// itemsAfter returns the number of loaded items that come after the cursor's
// line in the order of the groups, whether they show or not.
func (m Model[T]) itemsAfter() int {
	if m.sel < 0 || m.sel >= len(m.lines) {
		return 0
	}
	l := m.lines[m.sel]
	if l.more {
		return 0
	}
	after := m.nitems - l.before
	if !l.head {
		after--
	}
	return after
}

// countText reads the loaded items of a group, with "+" while more may come.
func (m Model[T]) countText(l line) string {
	n := strconv.Itoa(l.count)
	if !m.done {
		n += "+"
	}
	return " (" + n + ")"
}

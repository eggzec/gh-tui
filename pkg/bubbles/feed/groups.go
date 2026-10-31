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
	// kids is the number of children of an item line, which is foldable when
	// it has some: path then identifies it and open whether its children show.
	// A child line has child set, k as its place among them, item as its
	// item's and path as its item's.
	kids  int
	child bool
	k     int
}

// cursorRef remembers what the cursor is on, to find it in new lines.
type cursorRef struct {
	valid bool
	head  bool
	more  bool
	child bool
	k     int
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
	m.closed, m.seen = nil, nil
	return m.reshape(ref)
}

// SetSort orders every loaded item by cmp, which returns a negative number
// when a goes first, and the items of pages that load later are merged into
// that order. Equal items keep the order the source gave them. A nil cmp
// leaves the source's order. The cursor stays on its item. Sorted, the feed
// counts lines like a grouped one does, though it has no headers unless
// groups are set, and the items of each group are sorted.
func (m *Model[T]) SetSort(cmp func(a, b T) int) tea.Cmd {
	ref := m.cursor()
	m.less = cmp
	return m.reshape(ref)
}

// Resort puts the items in order again, for when what cmp reads has changed.
// It also picks up a changed number of children, which is read only when the
// lines are built.
func (m *Model[T]) Resort() {
	if m.lined() {
		m.regroup(m.cursor())
	}
}

// reshape lays the rows out again after the groups or the sort changed, with
// the cursor back on ref.
func (m *Model[T]) reshape(ref cursorRef) tea.Cmd {
	m.anchored, m.seeking, m.fill = false, false, true
	if !m.lined() {
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
	return len(m.groups) > 0 || m.less != nil
}

// lined reports whether the rows are lines: grouped or sorted, or with items
// that own children.
func (m Model[T]) lined() bool {
	return m.Grouped() || m.kids != nil
}

// foldable reports whether line l folds: a header, or an item with children.
func (l line) foldable() bool {
	return l.head || l.kids > 0
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
	if !m.lined() {
		return nil
	}
	ref := m.cursor()
	m.closed, m.fill = nil, true
	m.build()
	m.restore(ref)
	return m.sync()
}

// CollapseAll closes every group and every item's children, and moves the
// cursor to the header at the top of the group it was in. Without groups it
// moves to the item whose children closed.
func (m *Model[T]) CollapseAll() tea.Cmd {
	if !m.lined() {
		return nil
	}
	ref := m.headRef(0)
	if !ref.valid && m.sel >= 0 && m.sel < len(m.lines) && m.lines[m.sel].path != "" {
		// Without groups the cursor goes to the item whose children close.
		ref = m.itemRef(m.lines[m.sel].item)
	}
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
	if !m.lined() || m.sel < 0 || m.sel >= len(m.lines) {
		return nil
	}
	l := m.lines[m.sel]
	if !l.foldable() && open {
		return nil
	}
	ref, path := m.cursor(), l.path
	switch {
	case l.child:
		// The cursor goes to the item whose children close.
		ref = m.itemRef(l.item)
	case l.kids > 0:
	case !l.head:
		ref = m.headRef(l.depth - 1)
		path = ref.path
	}
	if path == "" || l.more {
		return nil
	}
	closed := maps.Clone(m.closed)
	if closed == nil {
		closed = map[string]struct{}{}
	}
	if open {
		delete(closed, path)
	} else {
		closed[path] = struct{}{}
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
	if !m.onFold() {
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
	if !m.lined() || m.sel < 0 || m.sel >= len(m.lines) {
		return nil
	}
	l := m.lines[m.sel]
	if l.more {
		return nil
	}
	if l.foldable() && l.open {
		return m.setOpen(false)
	}
	for i := m.sel - 1; i >= 0; i-- {
		if p := m.lines[i]; p.foldable() && p.depth < l.depth {
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
	return m.lined() && m.sel >= 0 && m.sel < len(m.lines) && m.lines[m.sel].head
}

// onFold reports whether the cursor is on a header or a foldable item.
func (m Model[T]) onFold() bool {
	return m.lined() && m.sel >= 0 && m.sel < len(m.lines) && m.lines[m.sel].foldable()
}

// Unfolded reports whether the cursor is on a header or an item that shows
// what is under it.
func (m Model[T]) Unfolded() bool {
	return m.onFold() && m.lines[m.sel].open
}

// UnfoldedItems returns the loaded items that show their children now, wherever
// the cursor is. An item inside a closed group doesn't show, so it is left out.
func (m Model[T]) UnfoldedItems() []T {
	var out []T
	for _, l := range m.lines {
		if l.head || l.more || l.child || !l.open {
			continue
		}
		if it, ok := m.item(l.item); ok {
			out = append(out, it)
		}
	}
	return out
}

// onChild reports whether the cursor is on a child row.
func (m Model[T]) onChild() bool {
	return m.lined() && m.sel >= 0 && m.sel < len(m.lines) && m.lines[m.sel].child
}

// onMore reports whether the cursor is on the row of the items not loaded.
func (m Model[T]) onMore() bool {
	return m.lined() && m.sel >= 0 && m.sel < len(m.lines) && m.lines[m.sel].more
}

// Activate is what enter does on the row of the items not loaded: it fetches
// the next chunk, if there is one to fetch. It reports whether the cursor was
// on that row, so the parent knows the key was taken, and does nothing for
// any other row.
func (m *Model[T]) Activate() (tea.Cmd, bool) {
	if m.onMore() {
		return m.fetchMore(), true
	}
	if m.kids != nil && m.sel >= 0 && m.sel < len(m.lines) {
		if l := m.lines[m.sel]; l.kids > 0 {
			return m.setOpen(!l.open), true
		}
	}
	return nil, false
}

// members returns the items the groups cover, in the order of the feed: those
// the quick filter keeps, or every loaded one without it.
func (m Model[T]) members() []int {
	items := m.rows
	if m.filter == "" {
		items = nil
		for i := range m.total {
			if _, ok := m.item(i); ok {
				items = append(items, i)
			}
		}
	}
	if m.less == nil {
		return items
	}
	items = slices.Clone(items)
	slices.SortStableFunc(items, func(a, b int) int {
		x, _ := m.item(a)
		y, _ := m.item(b)
		return m.less(x, y)
	})
	return items
}

// build makes the lines of the groups over the loaded items.
func (m *Model[T]) build() {
	m.lines, m.heads, m.order = nil, nil, nil
	if m.kids != nil && m.childrenFolded {
		// Copies of the model share the maps, so the first states go in new ones.
		m.closed, m.seen = maps.Clone(m.closed), maps.Clone(m.seen)
		if m.closed == nil {
			m.closed = map[string]struct{}{}
		}
		if m.seen == nil {
			m.seen = map[string]struct{}{}
		}
	}
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
			l := line{depth: level, item: i, before: *n}
			*n++
			if m.kids != nil {
				if it, ok := m.item(i); ok {
					l.kids = m.kids.count(it)
				}
			}
			if l.kids <= 0 {
				l.kids = 0
				if visible {
					m.lines = append(m.lines, l)
				}
				continue
			}
			l.path = parent + "\x1f" + m.itemKey(i)
			if m.seen != nil {
				if _, ok := m.seen[l.path]; !ok {
					m.seen[l.path] = struct{}{}
					m.closed[l.path] = struct{}{}
				}
			}
			_, closed := m.closed[l.path]
			l.open = !closed
			// Like a header's, the path of an item's fold is kept even inside a
			// closed group, so that toggleAll counts it.
			m.heads = append(m.heads, l.path)
			if !visible {
				continue
			}
			m.lines = append(m.lines, l)
			for k := 0; l.open && k < l.kids; k++ {
				m.lines = append(m.lines, line{
					child: true, k: k, depth: level + 1, item: i, path: l.path, before: *n,
				})
			}
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
	// Groups sort by the label the user sees, not by the key behind it.
	shown := func(k string) string {
		if g.Label != nil {
			return strings.ToLower(g.Label(k))
		}
		return strings.ToLower(k)
	}
	slices.SortStableFunc(keys, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "":
			return 1
		case b == "":
			return -1
		case g.Desc:
			return strings.Compare(shown(b), shown(a))
		}
		return strings.Compare(shown(a), shown(b))
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

// itemKey returns what identifies item i among the others in the path of its
// fold: its key, or its index without one.
func (m Model[T]) itemKey(i int) string {
	if it, ok := m.item(i); ok && m.key != nil {
		return m.key(it)
	}
	return strconv.Itoa(i)
}

// cursor returns what the cursor is on.
func (m Model[T]) cursor() cursorRef {
	if !m.lined() {
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
	ref := m.itemRef(m.lines[m.sel].item)
	if l := m.lines[m.sel]; l.child {
		ref.child, ref.k = true, l.k
	}
	return ref
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
		if l.head || l.more || l.child != ref.child || l.child && l.k != ref.k {
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
	if ref.child {
		// The child is gone, or its item is closed: the cursor stays on the
		// item.
		ref.child = false
		return m.restore(ref)
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
	if !l.head && !l.child {
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

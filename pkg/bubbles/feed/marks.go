package feed

import "maps"

// canMark reports whether the rows can be marked: the items have a key
// ([WithKey]) and a key is bound to [MarkKeys.Mark].
func (m Model[T]) canMark() bool {
	return m.key != nil && m.markKeys.Mark.Enabled()
}

// Gutter returns the width of what the feed draws left of every row: the
// cursor, and a cell for the mark where the rows can be marked. A parent
// that lays out the columns of its rows does so in the width less this.
func (m Model[T]) Gutter() int {
	if m.canMark() {
		return markedWidth
	}
	return gutterWidth
}

// Marks returns the number of marked items, those a quick filter hides
// included.
func (m Model[T]) Marks() int { return len(m.marks) }

// Marked reports whether item is marked.
func (m Model[T]) Marked(item T) bool { return m.marked(item) }

func (m Model[T]) marked(item T) bool {
	if len(m.marks) == 0 || m.key == nil {
		return false
	}
	return m.has(m.key(item))
}

func (m Model[T]) has(key string) bool {
	_, ok := m.marks[key]
	return ok
}

// cancels reports whether the cancel key has a find, a filter or a mark to
// clear.
func (m Model[T]) cancels() bool {
	return m.query != "" || m.filter != "" || len(m.marks) > 0
}

// toggleMark marks the selected item, or unmarks it if it is marked.
func (m *Model[T]) toggleMark() {
	it, ok := m.Selected()
	if !ok {
		return
	}
	k := m.key(it)
	// Copies of the model share the map, so change a copy of it.
	marks := maps.Clone(m.marks)
	if marks == nil {
		marks = map[string]struct{}{}
	}
	if _, on := marks[k]; on {
		delete(marks, k)
	} else {
		marks[k] = struct{}{}
	}
	if len(marks) == 0 {
		marks = nil
	}
	m.marks = marks
}

// MarkedItems returns the marked items that are loaded, in the order of the
// list and whether a quick filter shows them or not, and how many marked
// items are not among them: those in a chunk not loaded, or gone from the
// list. A parent that acts on the marked items acts on these, and says how
// many it left out.
func (m Model[T]) MarkedItems() (items []T, missing int) {
	if len(m.marks) == 0 || m.key == nil {
		return nil, 0
	}
	seen := make(map[string]struct{}, len(m.marks))
	for _, c := range m.chunks {
		if !c.loaded {
			continue
		}
		for _, it := range c.items {
			k := m.key(it)
			if !m.has(k) {
				continue
			}
			// An item can sit in two chunks while the list shifts.
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			items = append(items, it)
		}
	}
	return items, max(len(m.marks)-len(items), 0)
}

// ClearMarks unmarks every item.
func (m *Model[T]) ClearMarks() { m.marks = nil }

// SetMarks marks the items with the given keys and no others, such as to
// leave marked only the items that a change on all of them failed on,
// which a reload may have dropped for a moment. A mark of an item that is
// not in the list is forgotten once the whole list is loaded.
func (m *Model[T]) SetMarks(keys ...string) {
	var marks map[string]struct{}
	if len(keys) > 0 && m.key != nil {
		marks = make(map[string]struct{}, len(keys))
		for _, k := range keys {
			marks[k] = struct{}{}
		}
	}
	m.marks = marks
}

// pruneMarks forgets the marks of items that are gone. It can tell only
// when every item of the list is loaded and none is being fetched again:
// an item that the loaded ones lack may be in a chunk that is not loaded,
// or may have moved to a page not fetched yet.
func (m *Model[T]) pruneMarks() {
	if len(m.marks) == 0 || m.key == nil || !m.done || m.tail.fetching {
		return
	}
	present := make(map[string]struct{}, len(m.marks))
	for _, c := range m.chunks {
		if !c.loaded || c.fetching {
			return
		}
		for _, it := range c.items {
			if k := m.key(it); m.has(k) {
				present[k] = struct{}{}
			}
		}
	}
	if len(present) == len(m.marks) {
		return
	}
	if len(present) == 0 {
		present = nil
	}
	m.marks = present
}

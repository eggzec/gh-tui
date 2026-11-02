package conv

import "github.com/eggzec/gh-tui/internal/core"

// Reveal puts the cursor of the conversation on the row of the comment, review
// or thread comment with this ID, opening the folds above it. A comment inside
// a closed run lands on the run's row, which stays closed. It reports false
// when no row shows the item, as the filter leaves the rows.
func (m *Model) Reveal(itemID string) bool {
	if !m.Ready() {
		return false
	}
	v := m.views[""]
	id := rowOf(v.nodes, itemID)
	if id == "" {
		id = threadRowOf(v.nodes, itemID)
	}
	if id == "" {
		return false
	}
	if v.nodes[id].kind == KindRunComment {
		for rid, n := range v.nodes {
			if n.kind != KindRun || !v.tree.Folded(rid) {
				continue
			}
			for _, c := range n.children {
				if c.id == id {
					id = rid
				}
			}
		}
	}
	found, _ := v.tree.Reveal(id)
	return found
}

// Ready reports whether the rows are built, which takes the first page of the
// timeline and of the threads. Until then Reveal finds nothing, whatever the
// timeline holds.
func (m *Model) Ready() bool {
	v := m.views[""]
	return v != nil && v.done
}

// threadRowOf returns the ID of the row of the thread comment with ID c, or "".
func threadRowOf(nodes map[string]*node, c string) string {
	for id, n := range nodes {
		if (n.kind == KindThread || n.kind == KindReply) && n.comment != nil && n.comment.ID == c {
			return id
		}
	}
	return ""
}

// LatestBy returns the ID of the latest comment or review by login, as the
// timeline orders them, whatever the filter shows. A comment of a thread counts
// at the place of its review, and loses a tie against the review itself.
func (m *Model) LatestBy(login string) (itemID string, ok bool) {
	best, inThread := -1, false
	at := make(map[string]int, len(m.items))
	for i := range m.items {
		it := &m.items[i]
		if !kept(it) {
			continue
		}
		at[it.ID] = i
		if it.Actor.Login == login && i > best {
			best, itemID, inThread = i, it.ID, false
		}
	}
	for i := range m.threads {
		t := &m.threads[i]
		pos, attached := at[t.ReviewID]
		if !attached {
			continue
		}
		for j := range t.Comments {
			c := &t.Comments[j]
			if c.Author.Login == login && !c.Pending && (pos > best || pos == best && inThread) {
				best, itemID, inThread = pos, c.ID, true
			}
		}
	}
	return itemID, best >= 0
}

// LatestReviewBy returns the ID of the latest submitted review by login that
// is in state, as the timeline orders them, whatever the filter shows.
func (m *Model) LatestReviewBy(login string, state core.ReviewState) (itemID string, ok bool) {
	for i := range m.items {
		it := &m.items[i]
		if it.Kind == core.TimelineReview && kept(it) && it.Actor.Login == login && it.Review == state {
			itemID, ok = it.ID, true
		}
	}
	return itemID, ok
}

// Holds reports whether the timeline read so far has the comment, review or
// thread comment with this ID, whatever the filter shows.
func (m *Model) Holds(itemID string) bool {
	for i := range m.items {
		if it := &m.items[i]; kept(it) && it.ID == itemID {
			return true
		}
	}
	for i := range m.threads {
		for j := range m.threads[i].Comments {
			if c := &m.threads[i].Comments[j]; c.ID == itemID && !c.Pending {
				return true
			}
		}
	}
	return false
}

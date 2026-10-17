package conv

import (
	"cmp"
	"slices"
	"time"
)

// Order is how the conversation is sorted.
type Order int

// The orders. The zero one is the order of GitHub's timeline.
const (
	// OrderOldest keeps the order of the timeline: the oldest first.
	OrderOldest Order = iota
	// OrderNewest reverses it: the newest first.
	OrderNewest
	// OrderActivity puts first what was last written in, whether it is a
	// comment, or a reply in a review that is old.
	OrderActivity
)

// SetOrder sets how the conversation is sorted and builds the rows again. The
// cursor and the folds stay with their rows.
func (m *Model) SetOrder(o Order) {
	if m.order == o {
		return
	}
	m.order = o
	m.shape++
	m.sync()
}

// Order returns how the conversation is sorted.
func (m Model) Order() Order { return m.order }

// sortNodes sorts the top-level nodes, which are already in the order of the
// timeline, as units: a run keeps its comments and a review its threads in
// the order they have. The sort is stable, so that what ties keeps the
// timeline's order.
func sortNodes(out []*node, o Order) {
	switch o {
	case OrderOldest:
	case OrderNewest:
		slices.Reverse(out)
	case OrderActivity:
		at := make(map[*node]time.Time, len(out))
		for _, n := range out {
			at[n] = lastActivity(n)
		}
		slices.SortStableFunc(out, func(a, b *node) int { return cmp.Compare(at[b].UnixNano(), at[a].UnixNano()) })
	}
}

// lastActivity returns the time of the last thing written in n: for a comment,
// when it was written or edited; for a run, its last comment; for a review,
// the review itself or the latest comment of its threads; for a system line,
// its latest event.
func lastActivity(n *node) time.Time {
	switch n.kind {
	case KindRun:
		if len(n.children) == 0 {
			return time.Time{}
		}
		return lastActivity(n.children[len(n.children)-1])
	case KindEvent:
		var t time.Time
		for _, it := range n.events {
			t = latest(t, it.At)
		}
		return t
	case KindReview:
		var t time.Time
		if n.item != nil {
			t = n.item.At
		}
		for _, th := range n.children {
			for i := range th.thread.Comments {
				t = latest(t, th.thread.Comments[i].At)
			}
		}
		return t
	}
	if n.item == nil {
		return time.Time{}
	}
	return latest(n.item.At, n.item.UpdatedAt)
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

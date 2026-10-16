package conv

import "github.com/eggzec/gh-tui/internal/core"

// node is one entry of the tree of a conversation. Its children are drawn
// one level deeper.
type node struct {
	kind string
	id   string
	// item is the comment or review of the timeline.
	item *core.TimelineItem
	// thread is the thread of a thread node, and of its replies; comment is
	// the comment a thread or a reply node draws.
	thread  *core.Thread
	comment *core.ThreadComment
	// pending is the review that holds the viewer's drafts.
	pending  bool
	children []*node
}

// pendingID names the review of the drafts that GitHub has no review for.
const pendingID = "pending"

// drawn reports whether an item of the timeline has a row. An event that
// has none doesn't split a run of comments, and the unsubmitted review of
// the viewer is drawn at the end instead of where it was started.
func drawn(it *core.TimelineItem) bool {
	if it.Kind == core.TimelineReview {
		return it.Review != core.ReviewStatePending
	}
	return it.Kind == core.TimelineComment
}

// tree returns the top-level nodes of d, in the order of its timeline.
func tree(d Data) []*node {
	byReview := make(map[string][]*node)
	for i := range d.Threads {
		if t := &d.Threads[i]; len(t.Comments) > 0 && t.ReviewID != "" {
			byReview[t.ReviewID] = append(byReview[t.ReviewID], threadNode(t))
		}
	}

	var out, run []*node
	flush := func() {
		if len(run) > 1 {
			for _, c := range run {
				c.kind, c.id = KindRunComment, "rc:"+c.item.ID
			}
			out = append(out, &node{kind: KindRun, id: "run:" + run[0].item.ID, children: run})
		} else {
			out = append(out, run...)
		}
		run = nil
	}
	attached := make(map[string]bool)
	for i := range d.Items {
		it := &d.Items[i]
		if !drawn(it) {
			continue
		}
		if it.Kind == core.TimelineComment {
			run = append(run, &node{kind: KindComment, id: "c:" + it.ID, item: it})
			continue
		}
		flush()
		attached[it.ID] = true
		out = append(out, &node{kind: KindReview, id: "r:" + it.ID, item: it, children: byReview[it.ID]})
	}
	flush()

	if p := pendingReview(d, attached); p != nil {
		out = append(out, p)
	}
	return out
}

// pendingReview returns the review of the threads that were started in a
// review that is not submitted, which GitHub shows only to its author, or
// nil when there are none.
func pendingReview(d Data, attached map[string]bool) *node {
	var p *node
	for i := range d.Threads {
		t := &d.Threads[i]
		if len(t.Comments) == 0 || !t.Pending || attached[t.ReviewID] {
			continue
		}
		if p == nil {
			id := pendingID
			if t.ReviewID != "" {
				id = t.ReviewID
			}
			p = &node{kind: KindReview, id: "r:" + id, pending: true}
		}
		p.children = append(p.children, threadNode(t))
	}
	return p
}

// threadNode returns the node of t: its first comment, over the rest as
// replies, one level deep however they answer each other.
func threadNode(t *core.Thread) *node {
	n := &node{kind: KindThread, id: "t:" + t.ID, thread: t, comment: &t.Comments[0]}
	for i := 1; i < len(t.Comments); i++ {
		c := &t.Comments[i]
		n.children = append(n.children, &node{kind: KindReply, id: "p:" + c.ID, thread: t, comment: c})
	}
	return n
}

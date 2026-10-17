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
	// events are the items of an event node, in order; item is the first.
	events []*core.TimelineItem
	// author tells the pusher of a push node from the next one; forced marks a
	// push that follows a force push.
	author string
	forced bool
	// forms are the lines of a push: closed, which is centred, and open.
	forms [2]string
}

// pendingID names the review of the drafts that GitHub has no review for.
const pendingID = "pending"

// drawn reports whether an item of the timeline has a row. An event that
// has none doesn't split a run of comments, and the unsubmitted review of
// the viewer is drawn at the end instead of where it was started.
func drawn(it *core.TimelineItem) bool {
	return kept(it) || isEvent(it.Kind) || isPush(it.Kind)
}

// kept reports whether a narrowed view may show an item: a comment or a
// review that is submitted. A narrowed view never shows events.
func kept(it *core.TimelineItem) bool {
	if it.Kind == core.TimelineReview {
		return it.Review != core.ReviewStatePending
	}
	return it.Kind == core.TimelineComment
}

// tree returns the top-level nodes of d, in the order of its timeline.
// With preview, a comment stands alone and is never gathered in a run.
func tree(d Data, o Options) []*node {
	preview := o.preview
	// A narrowed view keeps every group and the order of the timeline.
	hidden := func(k core.TimelineKind) bool {
		g, ok := groupOf(k)
		return ok && !preview && o.hide[g]
	}
	byReview := make(map[string][]*node)
	for i := range d.Threads {
		if t := &d.Threads[i]; len(t.Comments) > 0 && t.ReviewID != "" {
			byReview[t.ReviewID] = append(byReview[t.ReviewID], threadNode(t))
		}
	}

	var out, run []*node
	flush := func() {
		if len(run) > 1 && !preview {
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
	seen := make(map[string]int)
	var lines merger
	var pushes pushRun
	// lone puts a force push that no push followed in its place.
	lone := func(n *node) {
		if n == nil {
			return
		}
		flush()
		n.id = eventID(n.item, seen)
		out = append(out, n)
	}
	for i := range d.Items {
		it := &d.Items[i]
		if !drawn(it) || preview && (isEvent(it.Kind) || isPush(it.Kind)) {
			continue
		}
		if it.Kind == core.TimelineReview {
			attached[it.ID] = true
		}
		// A kind that is hidden leaves no gap, so comments that only it
		// separated join in a run.
		if hidden(it.Kind) {
			continue
		}
		if isPush(it.Kind) {
			lines.split()
			if it.Kind == core.TimelineForcePushed {
				lone(pushes.force(it))
				continue
			}
			if n, fresh := pushes.commit(it); fresh {
				flush()
				out = append(out, n)
			}
			continue
		}
		lone(pushes.end())
		if isEvent(it.Kind) {
			n, fresh := lines.event(it)
			if fresh {
				flush()
				n.id = eventID(it, seen)
				out = append(out, n)
			}
			continue
		}
		lines.split()
		if it.Kind == core.TimelineComment {
			run = append(run, &node{kind: KindComment, id: "c:" + it.ID, item: it})
			continue
		}
		flush()
		out = append(out, &node{kind: KindReview, id: "r:" + it.ID, item: it, children: byReview[it.ID]})
	}
	lone(pushes.end())
	flush()
	if !preview {
		sortNodes(out, o.order)
		out = withCI(out, o)
	}

	if p := pendingReview(d, attached); p != nil && !hidden(core.TimelineReview) {
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

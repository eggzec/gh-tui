package conv

import (
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// defaultWidth is the width of rows drawn with none given.
const defaultWidth = 80

// builder makes the rows of one conversation.
type builder struct {
	o     Options
	st    *Styles
	width int
	rows  []treeview.Node
	// index, when it is not nil, gets each node by its ID.
	index map[string]*node
}

// Build returns the rows for d, in order, with stable IDs: comments, reviews
// over their threads and threads over their replies, with one blank row
// between the items of the top level.
func Build(d Data, o Options) []treeview.Node {
	rows, _ := build(d, o, false)
	return rows
}

// build is Build, which also returns the nodes the rows were made from by
// their IDs when index is set.
func build(d Data, o Options, index bool) (rows []treeview.Node, nodes map[string]*node) {
	b := &builder{o: o, st: &o.Styles, width: o.Width}
	if b.width <= 0 {
		b.width = defaultWidth
	}
	if index {
		b.index = make(map[string]*node)
	}
	for i, n := range tree(d, o) {
		// The line of the checks of a push sits right under it.
		if i > 0 && n.kind != KindCI {
			b.rows = append(b.rows, treeview.Node{ID: "b:" + n.id, Lines: []string{""}, Skip: true, Kind: KindBlank})
		}
		b.node(n, 0)
	}
	return b.rows, b.index
}

// node adds the rows of n at depth, and of its children.
func (b *builder) node(n *node, depth int) {
	var r treeview.Node
	var s segs
	switch n.kind {
	case KindComment, KindRunComment:
		r, s = b.comment(n, depth)
	case KindReview:
		r, s = b.review(n, depth)
	case KindThread:
		r, s = b.thread(n, depth)
	case KindReply:
		r, s = b.reply(n, depth)
	case KindRun:
		r = b.run(n)
	case KindEvent:
		r = b.event(n)
	case KindPush:
		r = b.push(n)
	case KindCommit:
		r = b.commit(n)
	case KindCI:
		r = b.ci(n)
	}
	r.ID, r.Depth, r.Kind = n.id, depth, n.kind
	b.rows = append(b.rows, r)
	if b.index != nil {
		b.index[n.id] = n
	}
	b.rows = append(b.rows, b.segments(n.id, s, depth+1)...)
	if r.Branch && n.kind == KindCommit {
		b.rows = append(b.rows, b.o.kids(n.id+"/", n.item.Commit.OID, depth+1)...)
	}
	for _, c := range n.children {
		b.node(c, depth+1)
	}
}

// ageOf tells when at was, or "" for no time.
func (b *builder) ageOf(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return b.o.Dates.Short(at, b.o.Now)
}

// right returns the right edge of a row: pending, edited and the age, two
// spaces apart.
func (b *builder) right(at time.Time, edited, pending bool) string {
	var parts []string
	if pending {
		parts = append(parts, b.st.warn.Render("pending"))
	}
	if edited {
		parts = append(parts, b.st.dim.Render("edited"))
	}
	if age := b.ageOf(at); age != "" {
		parts = append(parts, b.st.dim.Render(age))
	}
	return strings.Join(parts, "  ")
}

// marker returns what a closed fold hides: the glyph, then what it is.
func (b *builder) marker(n int, one, many string) string {
	if n == 0 {
		return b.st.Glyphs.Folded
	}
	if n == 1 {
		return b.st.Glyphs.Folded + " 1 " + one
	}
	return b.st.Glyphs.Folded + " " + strconv.Itoa(n) + " " + many
}

// comment draws a comment of the conversation.
func (b *builder) comment(n *node, depth int) (treeview.Node, segs) {
	it := n.item
	right := b.right(it.At, it.UpdatedAt.After(it.At), false)
	lines, s := b.body(b.st.who(it.Actor, false), it.Body, depth, false, right)
	return treeview.Node{Lines: lines, Right: right}, s
}

// review draws a review, a fold when it holds threads.
func (b *builder) review(n *node, depth int) (treeview.Node, segs) {
	var r treeview.Node
	var prefix, body, right string
	if n.pending {
		prefix = b.st.review(core.ReviewStatePending) + " "
		if b.o.Viewer != "" {
			prefix += b.st.who(core.User{Login: b.o.Viewer}, false) + b.st.dim.Render(" pending review")
		} else {
			prefix += b.st.dim.Render("your pending review")
		}
	} else {
		it := n.item
		prefix = b.st.review(it.Review) + " " + b.st.who(it.Actor, false)
		body, right = it.Body, b.right(it.At, false, false)
	}
	r.Right = right
	r.Branch = len(n.children) > 0
	if r.Branch {
		r.Marker = b.marker(len(n.children), "thread", "threads")
		r.Closed = !openReview(n) && !b.o.preview
	}
	var s segs
	r.Lines, s = b.body(prefix, body, depth, r.Branch, right+r.Marker)
	return r, s
}

// openReview reports whether a review shows its threads at first: one that
// asks for changes, one of comments with a thread that isn't resolved, and
// the pending one.
func openReview(n *node) bool {
	if n.pending {
		return true
	}
	if n.item.Review == core.ReviewStateChangesRequested {
		return true
	}
	if n.item.Review != core.ReviewStateCommented {
		return false
	}
	return slices.ContainsFunc(n.children, func(c *node) bool { return !c.thread.Resolved })
}

// thread draws a review thread as its first comment, a fold when it has
// replies, or more to the comment than its first line, or replies that were
// not read. A resolved one starts closed, and quiet, as a draft is, with the
// glyph of a pending review.
func (b *builder) thread(n *node, depth int) (treeview.Node, segs) {
	t, c := n.thread, n.comment
	quiet := t.Resolved || c.Pending
	glyph := b.st.thread(t.Resolved)
	if c.Pending {
		glyph = b.st.review(core.ReviewStatePending)
	}
	prefix := glyph + " " + b.st.who(c.Author, quiet)
	if where := location(t, b.st.Glyphs.Cut); where != "" {
		prefix += b.st.dim.Render(" " + b.st.Glyphs.Dot + " " + where)
	}
	if t.Outdated {
		prefix += " " + b.st.outdated()
	}
	right := b.right(c.At, !c.EditedAt.IsZero(), c.Pending)
	replies := max(t.TotalComments, len(t.Comments)) - 1

	kids := len(n.children) > 0
	r := treeview.Node{Right: right, Closed: t.Resolved, Marker: b.marker(replies, "reply", "replies")}
	var s segs
	r.Lines, s = b.body(prefix, c.Body, depth, kids, right+r.Marker)
	r.Lines = b.withSnippet(r.Lines, t, c.DiffHunk, prefix, depth, kids)
	// Replies that were not read have no row to open, so a thread of one
	// line closes over their count.
	unread := replies > 0 && !kids
	r.Branch = kids || len(r.Lines) > 1 || unread
	switch {
	case !r.Branch:
		r.Marker, r.Closed = "", false
	case replies == 0 && !kids:
		r.Marker = b.st.Glyphs.Folded
	case unread && len(r.Lines) == 1:
		r.Closed = true
	}
	if quiet {
		r.Lines = b.quiet(r.Lines, prefix)
	}
	return r, s
}

// location names where a thread is: the file and the line, as "b.go:7".
func location(t *core.Thread, cut string) string {
	if t.Path == "" {
		return ""
	}
	name := termtext.Truncate(termtext.OneLine(path.Base(t.Path)), 2*maxName, cut)
	line := t.Line
	if line == 0 {
		line = t.OriginalLine
	}
	if t.File || line == 0 {
		return name
	}
	return name + ":" + strconv.Itoa(line)
}

// reply draws a comment of a thread after the first.
func (b *builder) reply(n *node, depth int) (treeview.Node, segs) {
	c := n.comment
	right := b.right(c.At, !c.EditedAt.IsZero(), c.Pending)
	prefix := b.st.who(c.Author, c.Pending)
	lines, s := b.body(prefix, c.Body, depth, false, right)
	if c.Pending {
		lines = b.quiet(lines, prefix)
	}
	return treeview.Node{Lines: lines, Right: right}, s
}

// run draws the fold over a run of comments: how many, by whom, and when.
func (b *builder) run(n *node) treeview.Node {
	var names []string
	seen := make(map[string]bool)
	for _, c := range n.children {
		if s := Name(c.item.Actor); !seen[s] {
			seen[s] = true
			names = append(names, termtext.Truncate(s, maxName, b.st.Glyphs.Cut))
		}
	}
	first, last := b.ageOf(n.children[0].item.At), b.ageOf(n.children[len(n.children)-1].item.At)
	right := first
	if last != first {
		right = first + " " + b.st.Glyphs.Dash + " " + last
	}
	head := b.st.bold.Render(strconv.Itoa(len(n.children))+" comments") +
		b.st.dim.Render(" "+b.st.Glyphs.Dot+" "+strings.Join(names, ", "))
	return treeview.Node{
		Lines:  []string{head},
		Right:  b.st.dim.Render(right),
		Marker: b.st.Glyphs.Folded,
		Branch: true,
		Closed: true,
	}
}

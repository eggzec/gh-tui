package conv

import (
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// defaultWidth is the width of rows drawn with none given.
const defaultWidth = 80

// builder makes the rows of one conversation.
type builder struct {
	o     Options
	st    *Styles
	width int
	rows  []foldtree.Row
	// index, when it is not nil, gets each node by its ID.
	index map[string]*node
}

// Build returns the rows for d, in order, with stable IDs: comments, reviews
// over their threads and threads over their replies, with one blank row
// between the items of the top level.
func Build(d Data, o Options) []foldtree.Row {
	rows, _ := build(d, o, false)
	return rows
}

// build is Build, which also returns the nodes the rows were made from by
// their IDs when index is set.
func build(d Data, o Options, index bool) (rows []foldtree.Row, nodes map[string]*node) {
	b := &builder{o: o, st: &o.Styles, width: o.Width}
	if b.width <= 0 {
		b.width = defaultWidth
	}
	if index {
		b.index = make(map[string]*node)
	}
	for i, n := range tree(d, o) {
		if i > 0 {
			b.rows = append(b.rows, foldtree.Row{ID: "b:" + n.id, Lines: []string{""}, Skip: true, Kind: KindBlank})
		}
		b.node(n, 0)
	}
	return b.rows, b.index
}

// node adds the rows of n at depth, and of its children.
func (b *builder) node(n *node, depth int) {
	var r foldtree.Row
	switch n.kind {
	case KindComment, KindRunComment:
		r = b.comment(n, depth)
	case KindReview:
		r = b.review(n, depth)
	case KindThread:
		r = b.thread(n, depth)
	case KindReply:
		r = b.reply(n, depth)
	case KindRun:
		r = b.run(n)
	}
	r.ID, r.Depth, r.Kind = n.id, depth, n.kind
	b.rows = append(b.rows, r)
	if b.index != nil {
		b.index[n.id] = n
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
		return b.st.Glyphs.Fold
	}
	if n == 1 {
		return b.st.Glyphs.Fold + " 1 " + one
	}
	return b.st.Glyphs.Fold + " " + strconv.Itoa(n) + " " + many
}

// comment draws a comment of the conversation.
func (b *builder) comment(n *node, depth int) foldtree.Row {
	it := n.item
	right := b.right(it.At, it.UpdatedAt.After(it.At), false)
	return foldtree.Row{
		Lines: b.message(b.st.who(it.Actor, false), it.Body, depth, false, right),
		Right: right,
	}
}

// review draws a review, a fold when it holds threads.
func (b *builder) review(n *node, depth int) foldtree.Row {
	var r foldtree.Row
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
	r.Fold = len(n.children) > 0
	if r.Fold {
		r.Marker = b.marker(len(n.children), "thread", "threads")
		r.Closed = !openReview(n) && !b.o.preview
	}
	r.Lines = b.message(prefix, body, depth, r.Fold, right+r.Marker)
	return r
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
func (b *builder) thread(n *node, depth int) foldtree.Row {
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
	r := foldtree.Row{Right: right, Closed: t.Resolved, Marker: b.marker(replies, "reply", "replies")}
	r.Lines = b.message(prefix, c.Body, depth, kids, right+r.Marker)
	// Replies that were not read have no row to open, so a thread of one
	// line closes over their count.
	unread := replies > 0 && !kids
	r.Fold = kids || len(r.Lines) > 1 || unread
	switch {
	case !r.Fold:
		r.Marker, r.Closed = "", false
	case replies == 0 && !kids:
		r.Marker = b.st.Glyphs.Fold
	case unread && len(r.Lines) == 1:
		r.Closed = true
	}
	if quiet {
		r.Lines = b.quiet(r.Lines, prefix)
	}
	return r
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
func (b *builder) reply(n *node, depth int) foldtree.Row {
	c := n.comment
	right := b.right(c.At, !c.EditedAt.IsZero(), c.Pending)
	prefix := b.st.who(c.Author, c.Pending)
	lines := b.message(prefix, c.Body, depth, false, right)
	if c.Pending {
		lines = b.quiet(lines, prefix)
	}
	return foldtree.Row{Lines: lines, Right: right}
}

// run draws the fold over a run of comments: how many, by whom, and when.
func (b *builder) run(n *node) foldtree.Row {
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
	return foldtree.Row{
		Lines:  []string{head},
		Right:  b.st.dim.Render(right),
		Marker: b.st.Glyphs.Fold,
		Fold:   true,
		Closed: true,
	}
}

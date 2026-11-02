// Package conv turns the timeline and the review threads of a pull request
// into the rows of its conversation: comments, reviews that fold over their
// threads, threads that fold over their replies, and runs of plain comments
// that fold into one line. It is pure: data in, [treeview.Node]s out. Loading,
// caching and the view belong to the host.
package conv

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// The kinds of rows, as [treeview.Node.Kind] says them.
const (
	// KindComment is a comment of the conversation that stands alone.
	KindComment = "comment"
	// KindReview is a review, or the pending one that holds the viewer's
	// drafts.
	KindReview = "review"
	// KindThread is a review thread, drawn as its first comment.
	KindThread = "thread"
	// KindReply is a comment of a thread after the first.
	KindReply = "reply"
	// KindRun is the fold over two or more plain comments in a row.
	KindRun = "run"
	// KindRunComment is a comment inside a run.
	KindRunComment = "run-comment"
	// KindBlank is the space between two items.
	KindBlank = "blank"
	// KindPush is the fold over the commits one author pushed in a row.
	KindPush = "push"
	// KindCommit is a commit of a push.
	KindCommit = "commit"
	// KindCI is the line with the state of the checks of a push.
	KindCI = "ci"
	// KindSegment is a row of what follows the text that leads a body: a
	// details block, its content and its end, and the text between blocks.
	// [Owner] names the row that holds the body.
	KindSegment = "segment"
)

// Data is what the rows are built from: the timeline in GitHub's order and
// the review threads.
type Data struct {
	Items   []core.TimelineItem
	Threads []core.Thread
}

// Options say how rows are drawn.
type Options struct {
	// Width is the cells of the tree the rows are drawn in.
	Width int
	// Styles draw the rows; the icon set is theirs.
	Styles Styles
	// MD renders the bodies, shared by every caller. Without it a body shows
	// as plain wrapped text.
	MD *markdown.Renderer
	// Dates tells ages, or dates in the user's format, relative to Now.
	Dates ui.Dates
	Now   time.Time
	// Viewer is the login of the viewer, whose unsubmitted review shows under
	// their name.
	Viewer string

	// hide is the groups that are left out, inverted from the model's shown so
	// that the zero Options shows every group; the model sets it, and a
	// preview ignores it.
	hide [NumGroups]bool
	// order is how the top level is sorted; a preview keeps the timeline's.
	order Order
	// checks is the state of the checks of each commit, by its OID in lower
	// case; a commit that is not in it shows no icon.
	checks map[string]core.CheckState
	// kids gives the rows of the checks of the commit sha, whose row has the
	// ID prefix without its slash, at depth. Commits fold over them when it is
	// set.
	kids func(prefix, sha string, depth int) []treeview.Node

	// preview draws a narrowed view: each comment is a row of its own, with no
	// run, and a review is open.
	preview bool
}

// Narrow picks what a narrowed view shows. A nil func keeps everything of
// its kind. Both are asked only of what is drawn. A thread shows only under a
// kept review, or in the pending group, so dropping a review drops its
// threads.
type Narrow struct {
	Item   func(core.TimelineItem) bool
	Thread func(core.Thread) bool
}

// filter returns d with only what n keeps.
func (n Narrow) filter(d Data) Data {
	var out Data
	for i := range d.Items {
		it := &d.Items[i]
		if kept(it) && (n.Item == nil || n.Item(*it)) {
			out.Items = append(out.Items, *it)
		}
	}
	for i := range d.Threads {
		t := &d.Threads[i]
		if n.Thread == nil || n.Thread(*t) {
			out.Threads = append(out.Threads, *t)
		}
	}
	return out
}

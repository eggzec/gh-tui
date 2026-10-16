// Package conv turns the timeline and the review threads of a pull request
// into the rows of its conversation: comments, reviews that fold over their
// threads, threads that fold over their replies, and runs of plain comments
// that fold into one line. It is pure: data in, [foldtree.Row]s out. Loading,
// caching and the view belong to the host.
package conv

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// The kinds of rows, as [foldtree.Row.Kind] says them.
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
}

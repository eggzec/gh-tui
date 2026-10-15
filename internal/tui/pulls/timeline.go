package pulls

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// The Conversation tab lists the comments of the pull request and its
// submitted reviews, in the order they were made. Comments are read a page
// at a time, and a page holds the reviews made up to its last comment, so
// that the reviews stay among the comments as pages come in.

// entry is one item of the conversation: a comment, or a review.
type entry struct {
	comment  core.Comment
	review   core.Review
	isReview bool
	// failure is why the reviews couldn't be read, for the entry that says so.
	failure error
}

// cursorSep parts the comments' cursor from the time that the page before
// ends at in the cursor of a page of entries. The time is digits, so the
// cursor is split at the last separator, and the comments' cursor, a URL,
// may hold one.
const cursorSep = "@"

// splitCursor returns the cursor of the comments and the time up to which
// the reviews were already shown, which is the zero time for the first page.
func splitCursor(cursor string) (string, time.Time) {
	inner, nano, ok := strings.CutLast(cursor, cursorSep)
	if !ok {
		return cursor, time.Time{}
	}
	n, err := strconv.ParseInt(nano, 10, 64)
	if err != nil {
		return inner, time.Time{}
	}
	return inner, time.Unix(0, n)
}

// joinCursor returns the cursor of the page after one whose comments end at
// last, or "" after the last page.
func joinCursor(next string, last time.Time) string {
	if next == "" {
		return ""
	}
	return next + cursorSep + strconv.FormatInt(last.UnixNano(), 10)
}

// fetchTimeline returns what the thread fetches its pages with: a page of
// the comments of pull request number of repo, with the reviews among them.
// The reviews are read whole, and the service keeps them. The comments are
// the page's content: if the reviews can't be read, the page shows them with
// a line that says so, at the head of the first page, and failed tells
// whether it was for want of an answer from GitHub, so the page can be read
// again then.
func fetchTimeline(svc Service, repo core.RepoRef, number int, failed *atomic.Bool) thread.Fetch[entry] {
	return func(ctx context.Context, cursor string) ([]entry, string, error) {
		inner, after := splitCursor(cursor)
		var (
			reviews []core.Review
			rerr    error
			wg      sync.WaitGroup
		)
		wg.Go(func() { reviews, rerr = readReviews(ctx, svc, repo, number) })
		q := commentsQuery(repo, number)
		q.Cursor = inner
		p, err := svc.Comments(ctx, q)
		wg.Wait()
		if err != nil {
			return nil, "", err
		}
		out := timeline(p.Items, p.Next != "", reviews, after)
		if rerr != nil && ctx.Err() == nil {
			if failed != nil {
				failed.Store(ui.Unreached(rerr))
			}
			if inner == "" {
				out = append([]entry{{failure: rerr}}, out...)
			}
		} else if failed != nil {
			failed.Store(false)
		}
		return out, joinCursor(p.Next, lastAt(p.Items, after)), nil
	}
}

// readReviews reads every page of the reviews of pull request number.
func readReviews(ctx context.Context, svc Service, repo core.RepoRef, number int) ([]core.Review, error) {
	var out []core.Review
	for q := (pulls.ReviewsQuery{Repo: repo, Number: number}); ; {
		p, err := svc.Reviews(ctx, q)
		if err != nil {
			return nil, err
		}
		out = append(out, p.Items...)
		if p.Next == "" {
			return out, nil
		}
		q.Cursor = p.Next
	}
}

// cachedReviews returns the reviews of pull request number from memory, and
// whether every page of them is there.
func cachedReviews(svc Service, repo core.RepoRef, number int) ([]core.Review, bool) {
	var out []core.Review
	for q := (pulls.ReviewsQuery{Repo: repo, Number: number}); ; {
		p, ok := svc.CachedReviews(q)
		if !ok {
			return nil, false
		}
		out = append(out, p.Items...)
		if p.Next == "" {
			return out, true
		}
		q.Cursor = p.Next
	}
}

// cachedFirst returns the first page of entries from memory, as
// fetchTimeline would, with the reviews if they are cached too, or without
// them. It reports false if the comments aren't cached.
func cachedFirst(svc Service, repo core.RepoRef, number int) ([]entry, string, bool) {
	p, ok := svc.CachedComments(commentsQuery(repo, number))
	if !ok {
		return nil, "", false
	}
	reviews, _ := cachedReviews(svc, repo, number)
	return timeline(p.Items, p.Next != "", reviews, time.Time{}), joinCursor(p.Next, lastAt(p.Items, time.Time{})), true
}

// lastAt returns when the last of comments was made, or after if there are
// none.
func lastAt(comments []core.Comment, after time.Time) time.Time {
	if len(comments) == 0 {
		return after
	}
	return comments[len(comments)-1].CreatedAt
}

// timeline merges the comments of a page with the reviews submitted after
// the time after. A page that more follow takes the reviews up to its last
// comment, and the last page takes the rest. Pending reviews are left out.
// At the same instant a comment comes before a review, and a review made at
// the last comment's instant stays on that page, while a page after it takes
// only the reviews made later than after, so no review shows twice or is lost.
func timeline(comments []core.Comment, more bool, reviews []core.Review, after time.Time) []entry {
	upto := lastAt(comments, after)
	var mine []core.Review
	for i := range reviews {
		r := &reviews[i]
		if r.State == core.ReviewStatePending || r.SubmittedAt.IsZero() || !r.SubmittedAt.After(after) {
			continue
		}
		if more && r.SubmittedAt.After(upto) {
			continue
		}
		mine = append(mine, *r)
	}
	slices.SortStableFunc(mine, func(a, b core.Review) int { return a.SubmittedAt.Compare(b.SubmittedAt) })
	out := make([]entry, 0, len(comments)+len(mine))
	for i := range comments {
		for len(mine) > 0 && mine[0].SubmittedAt.Before(comments[i].CreatedAt) {
			out, mine = append(out, entry{review: mine[0], isReview: true}), mine[1:]
		}
		out = append(out, entry{comment: comments[i]})
	}
	for i := range mine {
		out = append(out, entry{review: mine[i], isReview: true})
	}
	return out
}

// renderEntry renders a comment or a review of the conversation.
func (m *detailModal) renderEntry(e entry, width int) string {
	if e.failure != nil {
		text, hint := m.sayReviews(e.failure)
		return strings.Join(ui.ErrorLine(m.theme.Errors(m.icons), text, hint, width-len(gutter)), "\n")
	}
	if e.isReview {
		return m.renderReview(e.review, width)
	}
	return m.renderComment(e.comment, width)
}

// reviewVerdict returns the verdict of a review, spelled out and in its
// color.
func (st *styles) reviewVerdict(s core.ReviewState) string {
	switch s {
	case core.ReviewStateApproved:
		return st.approved + st.author.Render(" approved")
	case core.ReviewStateChangesRequested:
		return st.changes + st.author.Render(" changes requested")
	case core.ReviewStateDismissed:
		return st.age.Render("dismissed")
	default:
		return st.author.Render("commented")
	}
}

// threadCounts returns how many of the review threads read so far are of
// review id, and how many of those aren't resolved.
func (t *threadsState) threadCounts(id string) (total, open int) {
	for i := range t.all {
		if t.all[i].ReviewID != id {
			continue
		}
		total++
		if !t.all[i].Resolved {
			open++
		}
	}
	return total, open
}

// renderReview renders a review as its author, verdict and age, over its
// body, markdown rendered like a comment's, and how many threads it opened,
// behind a bar. A review with nothing to say is the line alone.
func (m *detailModal) renderReview(r core.Review, width int) string {
	st := &m.st
	var b strings.Builder
	b.WriteString(gutter)
	b.WriteString(m.avatars.Line(r.AvatarURL))
	b.WriteString(st.commenter.Render(ui.OneLine(r.Author.Login)))
	b.WriteString(" ")
	b.WriteString(st.reviewVerdict(r.State))
	b.WriteString(st.age.Render(st.ic.Separator + m.dates.Prose(r.SubmittedAt, m.now())))
	b.WriteString(m.newTag(r.SubmittedAt, r.Author.Login, true))
	bar := gutter + st.bar
	m.bodies.Add(r.ID, r.Body)
	if body := m.thread.Markdown(r.Body, markdown.Room(width, 2*len(gutter)+2)); body != "" {
		b.WriteByte('\n')
		b.WriteString(markdown.Indent(body, bar))
	}
	if total, open := m.th.threadCounts(r.ID); total > 0 {
		b.WriteByte('\n')
		b.WriteString(bar)
		b.WriteString(st.author.Render(plural(total, "thread") + ", " + strconv.Itoa(open) + " unresolved"))
	}
	return b.String()
}

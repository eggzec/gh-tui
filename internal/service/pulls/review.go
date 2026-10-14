package pulls

import (
	"context"
	"errors"
	"fmt"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/tray"
)

var (
	// ErrPendingReview is what SubmitReview's error wraps when the viewer
	// already has a pending review on the pull request, which GitHub allows
	// one of. Nothing was sent.
	ErrPendingReview = github.ErrPendingReview

	// errOutdatedDraft is what SubmitReview refuses a draft with that was
	// written on another commit than the one the review is on.
	errOutdatedDraft = errors.New("a draft was written on an older commit")
	// errReviewEvent is what SubmitReview refuses an event it doesn't know with.
	errReviewEvent = errors.New("unknown review event")
)

// SubmitReview sends drafts as one review of pull request number of repo, on
// its head commit head, with the summary body and the verdict event. A draft
// with a ThreadID is a reply to that thread; any other starts a thread on its
// line or range. The review is sent as one, or not at all: when GitHub
// refuses a step, nothing of it is left behind there.
//
// Unlike the other changes, it is not optimistic: the caller clears the
// drafts only once SubmitReview returns nil, and a failure changes no cache.
// It sends exactly the drafts it is given, so the caller decides what to do
// with those it can't send. A draft that says which commit it was written on
// must say head, since a review is on one commit; one that says none, as the
// drafts of an earlier version did, is sent as it is. A token that may not
// change the repository sends nothing.
//
// Once GitHub accepts the review, what is cached of the pull request's
// detail, review threads, reviews and comments is marked stale, and what an
// earlier session kept of them is dropped, so that the next read of each
// goes to GitHub. If the viewer already has a pending review, the error
// wraps ErrPendingReview.
func (s *Service) SubmitReview(ctx context.Context, repo core.RepoRef, number int, head string, event core.ReviewEvent, body string, drafts []tray.Draft) error {
	if err := s.refused(repo); err != nil {
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	var (
		threads []github.ReviewThreadDraft
		replies []github.ReviewReplyDraft
	)
	switch {
	case !event.Valid():
		return fmt.Errorf("submit review of pull %s#%d: %w %q", repo, number, errReviewEvent, event)
	case head == "":
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, errNoHead)
	}
	for i := range drafts {
		d := &drafts[i]
		if d.Commit != "" && d.Commit != head {
			return fmt.Errorf("submit review of pull %s#%d: %w (draft %s)", repo, number, errOutdatedDraft, d.ID)
		}
		if d.ThreadID != "" {
			replies = append(replies, github.ReviewReplyDraft{ThreadID: d.ThreadID, Body: d.Body})
			continue
		}
		threads = append(threads, github.ReviewThreadDraft{Path: d.Path, Side: d.Side, Line: d.Line, StartLine: d.StartLine, Body: d.Body})
	}

	id, err := s.pullID(ctx, repo, number)
	if err != nil {
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	if _, err := s.api.SubmitReview(ctx, id, head, event, body, threads, replies); err != nil {
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	s.reviewSubmitted(repo, number)
	return nil
}

// pullID returns the node ID of pull request number of repo, from the cached
// detail if there is one, else from GitHub.
func (s *Service) pullID(ctx context.Context, repo core.RepoRef, number int) (string, error) {
	if e, st := s.details.Get(detailKey(repo, number)); st != cache.Miss && e.Value.ID != "" {
		return e.Value.ID, nil
	}
	return s.api.PullRequestID(ctx, repo, number)
}

// reviewSubmitted makes the next read of everything that a review changes go
// to GitHub: the detail's review state and thread counts, the threads, the
// reviews and the comments (a review shows in the timeline). Entries a kept
// shelf holds are dropped, since one read within the TTL would be served as
// fresh, and the list's mark no longer vouches for them. The changed pull
// request's list pages are marked stale too, for its review decision.
func (s *Service) reviewSubmitted(repo core.RepoRef, number int) {
	key := detailKey(repo, number)
	s.seen.Delete(key)
	forget(s.threads, s.keptThreads, key)
	forget(s.reviews, s.keptReviews, key)
	forget(s.comments, s.keptComments, key)
	s.details.Invalidate(key)
	s.keptDetails.Delete(key)
	s.lists.InvalidateTag(repoTag(repo))
}

// forget marks stale every entry of c tagged tag and deletes it from shelf.
func forget[V any](c *cache.Cache[V], shelf *cache.Shelf[V], tag string) {
	for key := range c.TaggedEntries(tag) {
		shelf.Delete(key)
	}
	c.InvalidateTag(tag)
}

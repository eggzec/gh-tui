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

	// ErrPendingAppeared is what SubmitReview's error wraps, with
	// ErrPendingReview, when the viewer started a pending review after the
	// caller last read it (see PendingReview), such as on the web while the
	// user confirmed. Nothing was sent. The caller reads the pending review
	// again and asks once more, since the user decided on another set of
	// comments.
	ErrPendingAppeared = errors.New("a pending review appeared on this pull request")
	// ErrPendingReviewOutdated is what SubmitIntoPending refuses with when
	// the pending review is on another commit than the pull request's head:
	// a comment added to it lands on its commit, so the review would not be
	// one of the head. Nothing was sent. The user updates or discards the
	// review on GitHub.
	ErrPendingReviewOutdated = errors.New("the pending review is on an older commit than the head")

	// errOutdatedDraft is what SubmitReview refuses a draft with that was
	// written on another commit than the one the review is on.
	errOutdatedDraft = errors.New("a draft was written on an older commit")
	// errNoPending is what SubmitIntoPending refuses a review with that has
	// no ID.
	errNoPending = errors.New("no pending review to add to")
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
// goes to GitHub. If the viewer already has a pending review, which GitHub
// allows one of, the error wraps ErrPendingReview and ErrPendingAppeared,
// and the cached read of the pending review is marked stale: the caller
// reads it again, and then uses SubmitIntoPending.
func (s *Service) SubmitReview(ctx context.Context, repo core.RepoRef, number int, head string, event core.ReviewEvent, body string, drafts []tray.Draft) error {
	threads, replies, err := s.reviewParts(repo, number, head, event, drafts)
	if err != nil {
		return err
	}
	id, err := s.pullID(ctx, repo, number)
	if err != nil {
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	if _, err := s.api.SubmitReview(ctx, id, head, event, body, threads, replies); err != nil {
		if errors.Is(err, ErrPendingReview) {
			s.pendings.InvalidateTag(detailKey(repo, number))
			err = fmt.Errorf("%w: %w", ErrPendingAppeared, err)
		}
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	s.reviewSubmitted(repo, number)
	return nil
}

// SubmitIntoPending sends drafts as SubmitReview does, but into the viewer's
// pending review pending (as PendingReview read it), which holds comments
// that were made elsewhere: it adds the drafts to it and submits it, so the
// review is one, with those comments. The drafts are only those that are not
// in it already.
//
// If the review is on another commit than head, the error wraps
// ErrPendingReviewOutdated and nothing was sent. Unlike SubmitReview, a
// failure may leave some drafts added to the pending review on GitHub, which
// nothing deletes: it is read again, as the threads are, so that the next
// PendingReview shows them, and the caller drops from its drafts those that
// it shows. On success the pending review is gone, and so are the caches
// that SubmitReview drops.
func (s *Service) SubmitIntoPending(ctx context.Context, repo core.RepoRef, number int, head string, pending core.PendingReview, event core.ReviewEvent, body string, drafts []tray.Draft) error {
	threads, replies, err := s.reviewParts(repo, number, head, event, drafts)
	if err != nil {
		return err
	}
	switch {
	case pending.ID == "":
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, errNoPending)
	case pending.Commit != "" && pending.Commit != head:
		return fmt.Errorf("submit review of pull %s#%d: %w (on %.7s, the head is %.7s)", repo, number, ErrPendingReviewOutdated, pending.Commit, head)
	}
	id, err := s.pullID(ctx, repo, number)
	if err != nil {
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	key := detailKey(repo, number)
	if err := s.api.SubmitPendingReview(ctx, id, pending.ID, event, body, threads, replies); err != nil {
		// Steps before the failing one went through.
		s.pendings.InvalidateTag(key)
		forget(s.threads, s.keptThreads, key)
		return fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	s.reviewSubmitted(repo, number)
	return nil
}

// SendReview sends a review of pull request number of repo with the verdict
// event and the summary body, on head, and nothing else: no comments. If the
// viewer has a pending review, which holds comments made elsewhere, the
// review is that one, submitted with the verdict and the summary; else it is
// a new one. A pending review that appears while the review is sent, such as
// one started on the web, is read again and used, once. The errors are those
// of SubmitReview and SubmitIntoPending.
func (s *Service) SendReview(ctx context.Context, repo core.RepoRef, number int, head string, event core.ReviewEvent, body string) error {
	pending, found, err := s.PendingReview(ctx, repo, number)
	if err != nil {
		return err
	}
	if found {
		return s.SubmitIntoPending(ctx, repo, number, head, pending, event, body, nil)
	}
	err = s.SubmitReview(ctx, repo, number, head, event, body, nil)
	if !errors.Is(err, ErrPendingAppeared) {
		return err
	}
	pending, found, rerr := s.PendingReview(ctx, repo, number)
	if rerr != nil || !found {
		return err
	}
	return s.SubmitIntoPending(ctx, repo, number, head, pending, event, body, nil)
}

// reviewParts checks that a review of drafts on head with event can be sent,
// and splits the drafts into the threads that start and the replies. Its
// errors are those of the submission.
func (s *Service) reviewParts(repo core.RepoRef, number int, head string, event core.ReviewEvent, drafts []tray.Draft) (threads []github.ReviewThreadDraft, replies []github.ReviewReplyDraft, err error) {
	if err := s.refused(repo); err != nil {
		return nil, nil, fmt.Errorf("submit review of pull %s#%d: %w", repo, number, err)
	}
	switch {
	case !event.Valid():
		return nil, nil, fmt.Errorf("submit review of pull %s#%d: %w %q", repo, number, errReviewEvent, event)
	case head == "":
		return nil, nil, fmt.Errorf("submit review of pull %s#%d: %w", repo, number, errNoHead)
	}
	for i := range drafts {
		d := &drafts[i]
		if d.Commit != "" && d.Commit != head {
			return nil, nil, fmt.Errorf("submit review of pull %s#%d: %w (draft %s)", repo, number, errOutdatedDraft, d.ID)
		}
		if d.ThreadID != "" {
			replies = append(replies, github.ReviewReplyDraft{ThreadID: d.ThreadID, Body: d.Body})
			continue
		}
		threads = append(threads, github.ReviewThreadDraft{Path: d.Path, Side: d.Side, Line: d.Line, StartLine: d.StartLine, File: d.Line == 0, Body: d.Body})
	}
	return threads, replies, nil
}

// pendingRead is what the pending review cache holds: the viewer's pending
// review of a pull request, if there is one.
type pendingRead struct {
	Review core.PendingReview
	Found  bool
}

// PendingReview returns the viewer's pending review of pull request number of
// repo, with every comment of it, and false if there is none: one started on
// the web, for instance, which a review sent from here is added to. The read
// is kept in memory only, since the review is private and the web changes
// it: it is fresh for the pull requests' TTL, and marked stale by a refresh
// (Invalidate, Revalidate), by ErrPendingAppeared and by a failure of
// SubmitIntoPending. After a submission it reads as none.
func (s *Service) PendingReview(ctx context.Context, repo core.RepoRef, number int) (core.PendingReview, bool, error) {
	if err := s.refused(repo); err != nil {
		return core.PendingReview{}, false, fmt.Errorf("read pending review of pull %s#%d: %w", repo, number, err)
	}
	e, err := s.pendings.Fetch(ctx, detailKey(repo, number), func(ctx context.Context, _ cache.Entry[pendingRead], _ bool) (cache.Entry[pendingRead], error) {
		r, ok, err := s.api.PendingReview(ctx, repo, number)
		if err != nil {
			return cache.Entry[pendingRead]{}, err
		}
		return cache.Entry[pendingRead]{Value: pendingRead{Review: r, Found: ok}, Tags: tags(repo, number)}, nil
	})
	if err != nil {
		return core.PendingReview{}, false, fmt.Errorf("read pending review of pull %s#%d: %w", repo, number, err)
	}
	return e.Value.Review, e.Value.Found, nil
}

// CachedPendingReview returns what PendingReview read, fresh or stale,
// without a request, and whether it has read one.
func (s *Service) CachedPendingReview(repo core.RepoRef, number int) (core.PendingReview, bool) {
	r, ok := cached(s.pendings, detailKey(repo, number))
	return r.Review, ok && r.Found
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
// reviews, the comments and the timeline. Entries a kept shelf holds are
// dropped, since one read within the TTL would be served as fresh, and the
// list's mark no longer vouches for them. The changed pull request's list
// pages are marked stale too, for its review decision, and the cached pending
// review is replaced by none.
func (s *Service) reviewSubmitted(repo core.RepoRef, number int) {
	key := detailKey(repo, number)
	s.seen.Delete(key)
	forget(s.threads, s.keptThreads, key)
	forget(s.reviews, s.keptReviews, key)
	forget(s.comments, s.keptComments, key)
	forget(s.timelines, s.keptTimelines, key)
	s.details.Invalidate(key)
	s.pullCommits.InvalidateTag(key)
	s.keptDetails.Delete(key)
	s.lists.InvalidateTag(repoTag(repo))
	// The review that was sent was the viewer's one pending review, which
	// reads as gone, rather than stale, so that no reader is shown it.
	s.pendings.Set(key, cache.Entry[pendingRead]{Tags: tags(repo, number)})
}

// forget marks stale every entry of c tagged tag and deletes it from shelf.
func forget[V any](c *cache.Cache[V], shelf *cache.Shelf[V], tag string) {
	for key := range c.TaggedEntries(tag) {
		shelf.Delete(key)
	}
	c.InvalidateTag(tag)
}

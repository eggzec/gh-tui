package pulls

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// commentOpsAPI is the part of the client that edits a pull request's text,
// replies to its review threads, and edits and deletes its comments.
type commentOpsAPI interface {
	UpdatePullRequestText(ctx context.Context, pullID, title, body string) (core.PullRequest, time.Time, error)
	ReplyToReviewThread(ctx context.Context, threadID, reviewID, body string) (core.ThreadComment, error)
	EditIssueComment(ctx context.Context, id, body string) (core.Edited, error)
	EditReviewComment(ctx context.Context, id, body string) (core.Edited, error)
	EditReview(ctx context.Context, id, body string) (core.Edited, error)
	DeleteIssueComment(ctx context.Context, id string) error
	DeleteReviewComment(ctx context.Context, id string) error
}

var (
	// errCannotEdit is why a pull request that GitHub says the viewer may not
	// update is not edited.
	errCannotEdit = errors.New("you can't edit this pull request")
	// errUnsent is why a comment that GitHub hasn't confirmed is not
	// edited or deleted: it has no ID to send yet.
	errUnsent = errors.New("the comment isn't confirmed yet")
)

// Unconfirmed reports whether id is that of a comment that is shown before
// GitHub confirmed it, which can be neither edited nor deleted until it is.
func Unconfirmed(id string) bool {
	return strings.HasPrefix(id, pendingPrefix)
}

// patch returns items with f applied to the first one that match accepts, on a
// copy, since the slice is shared with whoever holds the cached value. It
// reports false, and returns items, when none matches.
func patch[T any](items []T, match func(T) bool, f func(*T)) ([]T, bool) {
	i := slices.IndexFunc(items, match)
	if i < 0 {
		return items, false
	}
	items = slices.Clone(items)
	f(&items[i])
	return items, true
}

// unkeep drops what the kept shelf holds under tag, so that a later session
// reads it again: the change it doesn't hold was only confirmed by GitHub's
// answer to the change, not by a read.
func unkeep[V any](c *cache.Cache[V], shelf *cache.Shelf[V], tag string) {
	for key := range c.TaggedEntries(tag) {
		shelf.Delete(key)
	}
}

// EditPull sets the title and the description of pull request number of repo.
// They show at once in the cached list rows and detail, the description with
// the time it was edited, and the returned Op sends them. When GitHub accepts
// them, it keeps the description it returns. A token that may not change the
// repository is refused, and so is a cached detail that says the viewer may
// not update the pull request, and nothing changes then. Without a cached
// detail, GitHub's refusal is what rolls the change back.
func (s *Service) EditPull(repo core.RepoRef, number int, title, body string) *optimistic.Op {
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("edit pull %s#%d: %w", repo, number, err))
	}
	key := detailKey(repo, number)
	if e, st := s.details.Get(key); st != cache.Miss && e.Value.Caps.Known && !e.Value.Caps.Update {
		return optimistic.Refused(fmt.Errorf("edit pull %s#%d: %w", repo, number, errCannotEdit))
	}
	var got core.PullRequest
	var at time.Time
	// Lists don't carry the description, so only the detail takes it here.
	undoBody, _ := s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
		d.Body, d.BodyEditedAt = body, s.now()
		return d
	})
	inner := s.changeDetail("edit", repo, number, func(pr *core.PullRequest) { pr.Title = title }, nil,
		func(ctx context.Context, id string) (core.PullRequest, error) {
			pr, edited, err := s.api.UpdatePullRequestText(ctx, id, title, body)
			if err != nil {
				return core.PullRequest{}, err
			}
			got, at = pr, edited
			// The rows of the lists hold no description.
			pr.Body = ""
			return pr, nil
		})
	return optimistic.New(func(ctx context.Context) error {
		if err := inner.Do(ctx); err != nil {
			return err
		}
		// reconcile keeps the description the detail had, so it is set here.
		s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
			d.Body = got.Body
			if at.IsZero() {
				at = s.now()
			}
			d.BodyEditedAt = at
			return d
		})
		if e, st := s.details.Get(key); st != cache.Miss {
			_ = s.keptDetails.Save(key, cache.Entry[core.PullRequestDetail]{Value: e.Value, Tags: e.Tags})
		}
		return nil
	}, undoBody, inner.Rollback)
}

// Reply adds a reply by author to review thread threadID of pull request
// number of repo. It shows at once at the end of the thread, as a comment for
// which Unconfirmed reports true and, if the viewer has a pending review,
// Pending is set, since the reply joins it. The returned Op reads the pending
// review and sends the reply into it, or at once if there is none; GitHub's
// refusal because there is one is read again and tried once more. When GitHub
// accepts it, the reply it returns takes the place of the one shown, and when
// it refuses it, the reply is taken out.
func (s *Service) Reply(repo core.RepoRef, number int, threadID, body, author string) *optimistic.Op {
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("reply on pull %s#%d: %w", repo, number, err))
	}
	key := detailKey(repo, number)
	_, joins := s.CachedPendingReview(repo, number)
	placeholder := core.ThreadComment{
		ID:      pendingPrefix + strconv.FormatUint(s.pending.Add(1), 10),
		Author:  core.User{Login: author},
		At:      s.now(),
		Body:    body,
		Pending: joins,
	}
	undo := s.threads.MutateTag(key, onThread(threadID, func(t *core.Thread) {
		if len(t.Comments) > 0 {
			placeholder.ReplyTo = t.Comments[0].ID
		}
		t.Comments = append(slices.Clip(slices.Clone(t.Comments)), placeholder)
		t.TotalComments++
	}))
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.sendReply(ctx, repo, number, threadID, body)
		if err != nil {
			return fmt.Errorf("reply on pull %s#%d: %w", repo, number, err)
		}
		// The reply shows in the timeline's review items, whose cached pages
		// lack it.
		forget(s.timelines, s.keptTimelines, key)
		s.threads.MutateTag(key, onThread(threadID, func(t *core.Thread) {
			t.Comments = slices.Clone(t.Comments)
			// A refetch may have dropped the placeholder or brought the reply.
			if i := slices.IndexFunc(t.Comments, func(c core.ThreadComment) bool { return c.ID == placeholder.ID }); i >= 0 {
				t.Comments[i] = got
			} else if !slices.ContainsFunc(t.Comments, func(c core.ThreadComment) bool { return c.ID == got.ID }) {
				t.Comments = append(t.Comments, got)
			}
		}))
		unkeep(s.threads, s.keptThreads, key)
		return nil
	}, undo)
}

// sendReply sends the reply into the viewer's pending review, which is read
// first, or at once if there is none. A refusal because a pending review
// exists, when none was read, reads it again and tries once more.
func (s *Service) sendReply(ctx context.Context, repo core.RepoRef, number int, threadID, body string) (core.ThreadComment, error) {
	review, found, err := s.PendingReview(ctx, repo, number)
	if err != nil {
		return core.ThreadComment{}, err
	}
	reviewID := ""
	if found {
		reviewID = review.ID
	}
	got, err := s.api.ReplyToReviewThread(ctx, threadID, reviewID, body)
	if err != nil && reviewID == "" && errors.Is(err, github.ErrPendingReview) {
		s.pendings.Invalidate(detailKey(repo, number))
		review, found, rerr := s.PendingReview(ctx, repo, number)
		if rerr != nil || !found {
			return core.ThreadComment{}, err
		}
		reviewID = review.ID
		got, err = s.api.ReplyToReviewThread(ctx, threadID, reviewID, body)
	}
	if err != nil {
		return core.ThreadComment{}, err
	}
	if reviewID != "" {
		// The review holds one more comment than it was read with.
		s.pendings.Invalidate(detailKey(repo, number))
	}
	return got, nil
}

// onThread returns the change of a cached page of threads that applies f to
// thread id, on copies of what it changes. A page without the thread is left.
func onThread(id string, f func(*core.Thread)) func(core.ThreadPage) (core.ThreadPage, bool) {
	return func(p core.ThreadPage) (core.ThreadPage, bool) {
		threads, ok := patch(p.Threads, func(t core.Thread) bool { return t.ID == id }, f)
		p.Threads = threads
		return p, ok
	}
}

// EditComment sets the body of comment id, a comment of the conversation of
// pull request number of repo, in the cached comments and timeline at once.
// The returned Op sends it, and when GitHub accepts it, the body it returns is
// stored with the time it was edited.
func (s *Service) EditComment(repo core.RepoRef, number int, id, body string) *optimistic.Op {
	if err := s.refusedComment(repo, "edit comment on pull %s#%d: %w", number, id); err != nil {
		return optimistic.Refused(err)
	}
	key := detailKey(repo, number)
	set := func(body string, at time.Time) func() {
		undoComments := s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
			items, ok := patch(p.Items, func(c core.Comment) bool { return c.ID == id }, func(c *core.Comment) {
				c.Body, c.UpdatedAt = body, at
			})
			p.Items = items
			return p, ok
		}))
		undoTimeline := s.timelines.MutateTag(key, onTimeline(id, body, at))
		return func() { undoTimeline(); undoComments() }
	}
	undo := set(body, s.now())
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.EditIssueComment(ctx, id, body)
		if err != nil {
			return fmt.Errorf("edit comment on pull %s#%d: %w", repo, number, err)
		}
		set(got.Body, editedAt(got, s.now()))
		unkeep(s.comments, s.keptComments, key)
		unkeep(s.timelines, s.keptTimelines, key)
		return nil
	}, undo)
}

// editedAt returns when e was edited, or now if GitHub gave no time.
func editedAt(e core.Edited, now time.Time) time.Time {
	if e.At.IsZero() {
		return now
	}
	return e.At
}

// onTimeline returns the change of a cached timeline page that sets the body
// and update time of item id.
func onTimeline(id, body string, at time.Time) func(core.Page[core.TimelineItem]) (core.Page[core.TimelineItem], bool) {
	return func(p core.Page[core.TimelineItem]) (core.Page[core.TimelineItem], bool) {
		items, ok := patch(p.Items, func(it core.TimelineItem) bool { return it.ID == id }, func(it *core.TimelineItem) {
			it.Body, it.UpdatedAt = body, at
		})
		p.Items = items
		return p, ok
	}
}

// EditReviewComment sets the body of comment id, the first comment or a reply
// of review thread threadID of pull request number of repo, in the cached
// threads at once. The returned Op sends it, and when GitHub accepts it, the
// body it returns is stored with the time it was edited.
func (s *Service) EditReviewComment(repo core.RepoRef, number int, threadID, id, body string) *optimistic.Op {
	if err := s.refusedComment(repo, "edit comment on pull %s#%d: %w", number, id); err != nil {
		return optimistic.Refused(err)
	}
	key := detailKey(repo, number)
	set := func(body string, at time.Time) func() {
		return s.threads.MutateTag(key, onThread(threadID, func(t *core.Thread) {
			t.Comments, _ = patch(t.Comments, func(c core.ThreadComment) bool { return c.ID == id }, func(c *core.ThreadComment) {
				c.Body, c.EditedAt = body, at
			})
		}))
	}
	undo := set(body, s.now())
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.EditReviewComment(ctx, id, body)
		if err != nil {
			return fmt.Errorf("edit comment on pull %s#%d: %w", repo, number, err)
		}
		set(got.Body, editedAt(got, s.now()))
		unkeep(s.threads, s.keptThreads, key)
		// The pending review lists its comments with their bodies.
		s.pendings.Invalidate(key)
		return nil
	}, undo)
}

// EditReviewBody sets the summary of review reviewID of pull request number
// of repo in the cached timeline and reviews at once. The returned Op sends
// it, and when GitHub accepts it, the body it returns is stored.
func (s *Service) EditReviewBody(repo core.RepoRef, number int, reviewID, body string) *optimistic.Op {
	if err := s.refusedComment(repo, "edit comment on pull %s#%d: %w", number, reviewID); err != nil {
		return optimistic.Refused(err)
	}
	key := detailKey(repo, number)
	set := func(body string, at time.Time) func() {
		undoReviews := s.reviews.MutateTag(key, func(p core.Page[core.Review]) (core.Page[core.Review], bool) {
			items, ok := patch(p.Items, func(r core.Review) bool { return r.ID == reviewID }, func(r *core.Review) { r.Body = body })
			p.Items = items
			return p, ok
		})
		undoTimeline := s.timelines.MutateTag(key, onTimeline(reviewID, body, at))
		return func() { undoTimeline(); undoReviews() }
	}
	undo := set(body, s.now())
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.EditReview(ctx, reviewID, body)
		if err != nil {
			return fmt.Errorf("edit comment on pull %s#%d: %w", repo, number, err)
		}
		set(got.Body, editedAt(got, s.now()))
		unkeep(s.reviews, s.keptReviews, key)
		unkeep(s.timelines, s.keptTimelines, key)
		// The review may be the pending one, which holds the summary.
		s.pendings.Invalidate(key)
		return nil
	}, undo)
}

// DeleteComment takes comment id, a comment of the conversation of pull
// request number of repo, out of the cached comments and timeline at once.
// The returned Op sends it. GitHub answers with nothing to reconcile with, so
// when it accepts it, the detail and the lists, which count the comment, are
// marked stale.
func (s *Service) DeleteComment(repo core.RepoRef, number int, id string) *optimistic.Op {
	if err := s.refusedComment(repo, "delete comment on pull %s#%d: %w", number, id); err != nil {
		return optimistic.Refused(err)
	}
	key := detailKey(repo, number)
	undoComments := s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
		i := slices.IndexFunc(p.Items, func(c core.Comment) bool { return c.ID == id })
		if i < 0 {
			return p, false
		}
		p.Items = slices.Delete(slices.Clone(p.Items), i, i+1)
		return p, true
	}))
	undoTimeline := s.timelines.MutateTag(key, func(p core.Page[core.TimelineItem]) (core.Page[core.TimelineItem], bool) {
		i := slices.IndexFunc(p.Items, func(it core.TimelineItem) bool { return it.ID == id })
		if i < 0 {
			return p, false
		}
		p.Items = slices.Delete(slices.Clone(p.Items), i, i+1)
		return p, true
	})
	return optimistic.New(func(ctx context.Context) error {
		if err := s.api.DeleteIssueComment(ctx, id); err != nil {
			return fmt.Errorf("delete comment on pull %s#%d: %w", repo, number, err)
		}
		unkeep(s.comments, s.keptComments, key)
		unkeep(s.timelines, s.keptTimelines, key)
		s.staleDetail(repo, number)
		return nil
	}, undoComments, undoTimeline)
}

// DeleteReviewComment takes comment id out of review thread threadID of pull
// request number of repo in the cached threads at once. Deleting the first
// comment of a thread leaves its replies, as GitHub does, and a thread left
// with no comment goes too. The returned Op sends it, and when GitHub accepts
// it, the detail's summary of the threads is marked stale.
func (s *Service) DeleteReviewComment(repo core.RepoRef, number int, threadID, id string) *optimistic.Op {
	if err := s.refusedComment(repo, "delete comment on pull %s#%d: %w", number, id); err != nil {
		return optimistic.Refused(err)
	}
	key := detailKey(repo, number)
	undo := s.threads.MutateTag(key, func(p core.ThreadPage) (core.ThreadPage, bool) {
		ti := slices.IndexFunc(p.Threads, func(t core.Thread) bool { return t.ID == threadID })
		if ti < 0 {
			return p, false
		}
		ci := slices.IndexFunc(p.Threads[ti].Comments, func(c core.ThreadComment) bool { return c.ID == id })
		if ci < 0 {
			return p, false
		}
		p.Threads = slices.Clone(p.Threads)
		t := &p.Threads[ti]
		t.Comments = slices.Delete(slices.Clone(t.Comments), ci, ci+1)
		// What answered the comment now answers nothing, and the next reply
		// goes to the first one left.
		for i := range t.Comments {
			if t.Comments[i].ReplyTo == id {
				t.Comments[i].ReplyTo = ""
			}
		}
		t.TotalComments = max(t.TotalComments-1, 0)
		if len(t.Comments) == 0 && !t.MoreComments {
			p.Threads = slices.Delete(p.Threads, ti, ti+1)
			p.Total = max(p.Total-1, 0)
		}
		return p, true
	})
	return optimistic.New(func(ctx context.Context) error {
		if err := s.api.DeleteReviewComment(ctx, id); err != nil {
			return fmt.Errorf("delete comment on pull %s#%d: %w", repo, number, err)
		}
		unkeep(s.threads, s.keptThreads, key)
		// A reply posted at once made a review of its own, which its deletion
		// may drop, and a pending review lists the comment.
		forget(s.timelines, s.keptTimelines, key)
		forget(s.reviews, s.keptReviews, key)
		s.pendings.Invalidate(key)
		s.staleDetail(repo, number)
		return nil
	}, undo)
}

// refusedComment returns why the comment id of pull request number of repo
// can't be changed, formatted by format with the repository, number and cause,
// or nil when it can: a token that may not write, or a comment that GitHub
// hasn't confirmed.
func (s *Service) refusedComment(repo core.RepoRef, format string, number int, id string) error {
	err := s.refused(repo)
	if err == nil && Unconfirmed(id) {
		err = errUnsent
	}
	if err == nil {
		return nil
	}
	return fmt.Errorf(format, repo, number, err)
}

// staleDetail makes the next read of the detail of pull request number of
// repo, and of the lists that show it, go to GitHub, since a count in them
// changed.
func (s *Service) staleDetail(repo core.RepoRef, number int) {
	key := detailKey(repo, number)
	s.seen.Delete(key)
	s.details.Invalidate(key)
	s.keptDetails.Delete(key)
	s.lists.InvalidateTag(repoTag(repo))
}

package pulls

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// opTimeline caches a timeline page of the items of pull request 1.
func opTimeline(s *Service, items ...core.TimelineItem) {
	s.timelines.Set(detailKey(repo, 1)+"/timeline", cache.Entry[core.Page[core.TimelineItem]]{
		Value: core.Page[core.TimelineItem]{Items: items}, Tags: tags(repo, 1),
	})
}

func timelineBodies(s *Service) []string {
	var out []string
	for _, e := range s.timelines.TaggedEntries(detailKey(repo, 1)) {
		for i := range e.Value.Items {
			it := &e.Value.Items[i]
			out = append(out, it.ID+":"+it.Body)
		}
	}
	return out
}

func TestEditPullCancelledRestoresTheDetail(t *testing.T) {
	s := opService(t, &fakeAPI{})
	s.EditPull(repo, 1, "New", "new body").Rollback()
	d, st := s.details.Get(detailKey(repo, 1))
	if d.Value.Title != "Old" || d.Value.Body != "old body" || !d.Value.BodyEditedAt.IsZero() || st == cache.Stale {
		t.Errorf("detail = %q, %q, %v (state %v), want it as it was, not stale", d.Value.Title, d.Value.Body, d.Value.BodyEditedAt, st)
	}
}

func TestEditPullKeepsTheNewBodyOnTheShelf(t *testing.T) {
	s := opService(t, &fakeAPI{}, WithStore(openStore(t)))
	if err := s.EditPull(repo, 1, "New", "new body").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if e, ok := s.keptDetails.Load(detailKey(repo, 1)); !ok || e.Value.Body != "new body" {
		t.Errorf("kept detail = %q (found %v), want the new body", e.Value.Body, ok)
	}
}

func TestEditCommentDropsTheKeptPages(t *testing.T) {
	s := opService(t, &fakeAPI{}, WithStore(openStore(t)))
	pages := s.comments.TaggedEntries(detailKey(repo, 1))
	keys := make([]string, 0, len(pages))
	for k := range pages {
		keys = append(keys, k)
		if _, ok := s.keptComments.Load(k); !ok {
			t.Fatalf("page %s was not kept before the edit", k)
		}
	}
	if err := s.EditComment(repo, 1, "c1", "new").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	for _, k := range keys {
		if _, ok := s.keptComments.Load(k); ok {
			t.Errorf("page %s is still kept, want it dropped", k)
		}
	}
}

func TestEditAndDeleteCommentOnTheTimeline(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	opTimeline(s, core.TimelineItem{ID: "c1", Body: "old"}, core.TimelineItem{ID: "c2", Body: "two"})

	edit := s.EditComment(repo, 1, "c1", "new")
	if got := timelineBodies(s); !slices.Equal(got, []string{"c1:new", "c2:two"}) {
		t.Fatalf("timeline before Do = %v", got)
	}
	api.opsErr = map[string]error{"EditIssueComment": errors.New("boom")}
	if err := edit.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if got := timelineBodies(s); !slices.Equal(got, []string{"c1:old", "c2:two"}) {
		t.Errorf("timeline after the failure = %v, want it rolled back", got)
	}

	del := s.DeleteComment(repo, 1, "c2")
	if got := timelineBodies(s); !slices.Equal(got, []string{"c1:old"}) {
		t.Fatalf("timeline before Do = %v", got)
	}
	api.opsErr = map[string]error{"DeleteIssueComment": errors.New("boom")}
	if err := del.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if got := timelineBodies(s); !slices.Equal(got, []string{"c1:old", "c2:two"}) {
		t.Errorf("timeline after the failure = %v, want it rolled back", got)
	}
}

func TestEditReviewCommentKeepsWhatGitHubReturns(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	api := &fakeAPI{}
	api.edited = map[string]core.Edited{"EditReviewComment": {ID: "t1b", Body: "as kept", At: at}}
	s := opService(t, api)
	if err := s.EditReviewComment(repo, 1, "t1", "t1b", "edited").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if th, _ := opThread(t, s, "t1"); th.Comments[1].Body != "as kept" || !th.Comments[1].EditedAt.Equal(at) {
		t.Errorf("reply = %+v, want GitHub's body and edit time", th.Comments[1])
	}
}

func TestEditReviewBodyKeepsWhatGitHubReturns(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	api := &fakeAPI{}
	api.edited = map[string]core.Edited{"EditReview": {ID: "r1", Body: "as kept", At: at}}
	s := opService(t, api)
	key := detailKey(repo, 1)
	s.reviews.Set(key+"/reviews", cache.Entry[core.Page[core.Review]]{Value: core.Page[core.Review]{Items: []core.Review{{ID: "r1", Body: "old"}}}, Tags: tags(repo, 1)})
	opTimeline(s, core.TimelineItem{ID: "r1", Body: "old"})
	if err := s.EditReviewBody(repo, 1, "r1", "new").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if e, _ := s.reviews.Get(key + "/reviews"); e.Value.Items[0].Body != "as kept" {
		t.Errorf("review body = %q, want GitHub's", e.Value.Items[0].Body)
	}
	for _, e := range s.timelines.TaggedEntries(key) {
		if it := e.Value.Items[0]; it.Body != "as kept" || !it.UpdatedAt.Equal(at) {
			t.Errorf("timeline item = %+v, want GitHub's body and time", it)
		}
	}
}

// TestReviewCommentOpsRefreshThePendingReview checks that what lists the
// pending review's comments and summary is read again after a change to them.
func TestReviewCommentOpsRefreshThePendingReview(t *testing.T) {
	ops := map[string]func(s *Service) error{
		"EditReviewComment": func(s *Service) error {
			return s.EditReviewComment(repo, 1, "t1", "t1b", "x").Do(t.Context())
		},
		"DeleteReviewComment": func(s *Service) error {
			return s.DeleteReviewComment(repo, 1, "t1", "t1b").Do(t.Context())
		},
		"EditReviewBody": func(s *Service) error { return s.EditReviewBody(repo, 1, "PRR_9", "x").Do(t.Context()) },
	}
	for name, do := range ops {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{pendingReview: core.PendingReview{ID: "PRR_9"}, pendingFound: true}
			s := opService(t, api)
			if _, _, err := s.PendingReview(t.Context(), repo, 1); err != nil {
				t.Fatal(err)
			}
			if _, st := s.pendings.Get(detailKey(repo, 1)); st != cache.Fresh {
				t.Fatalf("pending read state = %v before the change, want fresh", st)
			}
			if err := do(s); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if _, st := s.pendings.Get(detailKey(repo, 1)); st != cache.Stale {
				t.Errorf("pending read state = %v, want stale", st)
			}
		})
	}
}

func TestDeleteReviewCommentForgetsTheTimelineAndReviews(t *testing.T) {
	s := opService(t, &fakeAPI{})
	key := detailKey(repo, 1)
	opTimeline(s, core.TimelineItem{ID: "x"})
	s.reviews.Set(key+"/reviews", cache.Entry[core.Page[core.Review]]{Tags: tags(repo, 1)})
	if err := s.DeleteReviewComment(repo, 1, "t1", "t1b").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, st := s.timelines.Get(key + "/timeline"); st != cache.Stale {
		t.Errorf("timeline state = %v, want stale", st)
	}
	if _, st := s.reviews.Get(key + "/reviews"); st != cache.Stale {
		t.Errorf("reviews state = %v, want stale", st)
	}
}

func TestReplyRefusedForAPendingReviewFindingNone(t *testing.T) {
	api := &fakeAPI{}
	api.replyErrs = []error{github.ErrPendingReview}
	s := opService(t, api)
	err := s.Reply(repo, 1, "t2", "hi", "octocat").Do(t.Context())
	if !errors.Is(err, github.ErrPendingReview) {
		t.Fatalf("Do = %v, want the refusal", err)
	}
	if n := len(api.callsOf()); n != 1 {
		t.Errorf("%d replies were sent, want 1: there was no review to join", n)
	}
	if th, _ := opThread(t, s, "t2"); len(th.Comments) != 1 {
		t.Errorf("comments = %v, want the placeholder gone", threadBodies(th))
	}
}

func TestReplyRefusedAndTheReReadFails(t *testing.T) {
	api := &fakeAPI{}
	api.replyErrs = []error{github.ErrPendingReview}
	s := opService(t, api)
	if _, _, err := s.PendingReview(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	api.pendingErr = errors.New("offline")
	err := s.Reply(repo, 1, "t2", "hi", "octocat").Do(t.Context())
	if !errors.Is(err, github.ErrPendingReview) {
		t.Fatalf("Do = %v, want the original refusal", err)
	}
	if th, _ := opThread(t, s, "t2"); len(th.Comments) != 1 {
		t.Errorf("comments = %v, want the placeholder gone", threadBodies(th))
	}
}

func TestReplyFailsWhenThePendingReviewCantBeRead(t *testing.T) {
	api := &fakeAPI{pendingErr: errors.New("offline")}
	s := opService(t, api)
	err := s.Reply(repo, 1, "t2", "hi", "octocat").Do(t.Context())
	if err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if n := len(api.callsOf()); n != 0 {
		t.Errorf("%d replies were sent, want none", n)
	}
	if th, _ := opThread(t, s, "t2"); len(th.Comments) != 1 {
		t.Errorf("comments = %v, want the placeholder gone", threadBodies(th))
	}
}

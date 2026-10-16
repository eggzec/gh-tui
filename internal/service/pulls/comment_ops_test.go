package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// opThreads is a cached page of two threads: t1 with a first comment and a
// reply, and t2 with one comment.
func opThreads() core.ThreadPage {
	return core.ThreadPage{Total: 2, Threads: []core.Thread{
		{ID: "t1", TotalComments: 2, Comments: []core.ThreadComment{
			{ID: "t1a", Body: "first"}, {ID: "t1b", Body: "reply", ReplyTo: "t1a"},
		}},
		{ID: "t2", TotalComments: 1, Comments: []core.ThreadComment{{ID: "t2a", Body: "only"}}},
	}}
}

// opService returns a service whose caches hold the comments of comment_test's
// pages, the threads of opThreads and a detail of pull request 1.
func opService(t *testing.T, api *fakeAPI, opts ...Option) *Service {
	t.Helper()
	s := New(api, opts...)
	api.comments = twoPages
	readCommentPages(t, s)
	key := detailKey(repo, 1)
	s.threads.Set(key+"/threads", cache.Entry[core.ThreadPage]{Value: opThreads(), Tags: tags(repo, 1)})
	d := core.PullRequestDetail{}
	d.ID, d.Number, d.Title, d.Body = "PR_1", 1, "Old", "old body"
	d.Caps = core.ItemCaps{Known: true, Update: true}
	s.details.Set(key, cache.Entry[core.PullRequestDetail]{Value: d, Tags: tags(repo, 1)})
	return s
}

func opThread(t *testing.T, s *Service, id string) (core.Thread, bool) {
	t.Helper()
	for _, e := range s.threads.TaggedEntries(detailKey(repo, 1)) {
		for i := range e.Value.Threads {
			if e.Value.Threads[i].ID == id {
				return e.Value.Threads[i], true
			}
		}
	}
	return core.Thread{}, false
}

func threadBodies(th core.Thread) []string {
	out := make([]string, 0, len(th.Comments))
	for i := range th.Comments {
		out = append(out, th.Comments[i].ID+":"+th.Comments[i].Body)
	}
	return out
}

func TestEditCommentShowsAtOnceAndRollsBack(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	before, _ := s.CachedComments(firstPage)

	op := s.EditComment(repo, 1, "c1", "new")
	if p, _ := s.CachedComments(firstPage); p.Items[0].Body != "new" {
		t.Fatalf("body before Do = %q, want new", p.Items[0].Body)
	}
	if before.Items[0].Body != "" {
		t.Errorf("the snapshot taken before was changed to %q", before.Items[0].Body)
	}
	api.opsErr = map[string]error{"EditIssueComment": errors.New("boom")}
	err := op.Do(t.Context())
	if err == nil || !strings.Contains(err.Error(), "edit comment on pull eggzec/gh-tui#1") {
		t.Fatalf("Do = %v, want the wrapped error", err)
	}
	if p, _ := s.CachedComments(firstPage); p.Items[0].Body != "" {
		t.Errorf("body after the failure = %q, want it rolled back", p.Items[0].Body)
	}
}

func TestEditCommentKeepsWhatGitHubReturns(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	api := &fakeAPI{}
	api.edited = map[string]core.Edited{"EditIssueComment": {ID: "c1", Body: "as kept", At: at}}
	s := opService(t, api)
	if err := s.EditComment(repo, 1, "c1", "new").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if p, _ := s.CachedComments(firstPage); p.Items[0].Body != "as kept" || !p.Items[0].UpdatedAt.Equal(at) {
		t.Errorf("comment = %+v, want GitHub's body and edit time", p.Items[0])
	}
}

func TestEditReviewCommentChangesAReply(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	before, _ := opThread(t, s, "t1")

	op := s.EditReviewComment(repo, 1, "t1", "t1b", "edited")
	if th, _ := opThread(t, s, "t1"); !slices.Equal(threadBodies(th), []string{"t1a:first", "t1b:edited"}) || th.Comments[1].EditedAt.IsZero() {
		t.Fatalf("thread before Do = %v", threadBodies(th))
	}
	if before.Comments[1].Body != "reply" {
		t.Errorf("the snapshot taken before was changed")
	}
	api.opsErr = map[string]error{"EditReviewComment": errors.New("boom")}
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if th, _ := opThread(t, s, "t1"); th.Comments[1].Body != "reply" {
		t.Errorf("reply after the failure = %q, want it rolled back", th.Comments[1].Body)
	}
}

func TestEditReviewBodyChangesTheCachedReview(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	key := detailKey(repo, 1)
	s.reviews.Set(key+"/reviews", cache.Entry[core.Page[core.Review]]{Value: core.Page[core.Review]{Items: []core.Review{{ID: "r1", Body: "old"}}}, Tags: tags(repo, 1)})

	op := s.EditReviewBody(repo, 1, "r1", "new")
	e, _ := s.reviews.Get(key + "/reviews")
	if e.Value.Items[0].Body != "new" {
		t.Fatalf("review body before Do = %q", e.Value.Items[0].Body)
	}
	api.opsErr = map[string]error{"EditReview": errors.New("boom")}
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if e, _ := s.reviews.Get(key + "/reviews"); e.Value.Items[0].Body != "old" {
		t.Errorf("review body after the failure = %q, want old", e.Value.Items[0].Body)
	}
}

func TestDeleteCommentRemovesAtOnceAndRollsBack(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	before, _ := s.CachedComments(firstPage)

	op := s.DeleteComment(repo, 1, "c2")
	if got := commentIDs(t, s, firstPage); !slices.Equal(got, []string{"c1"}) {
		t.Fatalf("first page before Do = %q, want c1", got)
	}
	if len(before.Items) != 2 {
		t.Errorf("the snapshot taken before has %d items, want 2", len(before.Items))
	}
	api.opsErr = map[string]error{"DeleteIssueComment": errors.New("boom")}
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if got := commentIDs(t, s, firstPage); !slices.Equal(got, []string{"c1", "c2"}) {
		t.Errorf("first page after the failure = %q, want it rolled back", got)
	}
}

func TestDeleteCommentMarksTheDetailStale(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	if err := s.DeleteComment(repo, 1, "c1").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, st := s.details.Get(detailKey(repo, 1)); st != cache.Stale {
		t.Errorf("detail state = %v, want stale", st)
	}
}

func TestDeleteReviewCommentKeepsTheRepliesOfAFirstComment(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	before, _ := opThread(t, s, "t1")

	op := s.DeleteReviewComment(repo, 1, "t1", "t1a")
	th, ok := opThread(t, s, "t1")
	if !ok || !slices.Equal(threadBodies(th), []string{"t1b:reply"}) || th.TotalComments != 1 {
		t.Fatalf("thread before Do = %v (found %v), want the reply to stay", threadBodies(th), ok)
	}
	if len(before.Comments) != 2 {
		t.Errorf("the snapshot taken before was changed")
	}
	if th.Comments[0].ReplyTo != "" {
		t.Errorf("the reply still answers %q, want nothing", th.Comments[0].ReplyTo)
	}
	api.opsErr = map[string]error{"DeleteReviewComment": errors.New("boom")}
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if th, _ := opThread(t, s, "t1"); len(th.Comments) != 2 || th.Comments[1].ReplyTo != "t1a" {
		t.Errorf("comments after the failure = %v, want both back", threadBodies(th))
	}
}

func TestDeleteTheLastCommentRemovesTheThread(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	op := s.DeleteReviewComment(repo, 1, "t2", "t2a")
	if _, ok := opThread(t, s, "t2"); ok {
		t.Fatal("thread t2 is still cached, want it removed")
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, st := s.details.Get(detailKey(repo, 1)); st != cache.Stale {
		t.Errorf("detail state = %v, want stale for its thread summary", st)
	}
}

func TestReplyJoinsAPendingReview(t *testing.T) {
	api := &fakeAPI{pendingReview: core.PendingReview{ID: "PRR_9"}, pendingFound: true}
	api.reply = core.ThreadComment{ID: "real", Body: "hi"}
	s := opService(t, api)
	if _, ok, err := s.PendingReview(t.Context(), repo, 1); err != nil || !ok {
		t.Fatalf("PendingReview = %v, %v", ok, err)
	}

	before, _ := opThread(t, s, "t2")
	op := s.Reply(repo, 1, "t2", "hi", "octocat")
	th, _ := opThread(t, s, "t2")
	if len(th.Comments) != 2 {
		t.Fatalf("comments before Do = %v, want the placeholder", threadBodies(th))
	}
	if c := th.Comments[1]; !Unconfirmed(c.ID) || !c.Pending || c.ReplyTo != "t2a" || c.Author.Login != "octocat" {
		t.Errorf("placeholder = %+v, want pending, unconfirmed, replying to t2a", c)
	}
	if len(before.Comments) != 1 {
		t.Errorf("the snapshot taken before was changed")
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := api.callsOf(); len(got) != 1 || got[0].args[1] != "PRR_9" {
		t.Errorf("calls = %+v, want one reply into PRR_9", got)
	}
	if th, _ := opThread(t, s, "t2"); th.Comments[1].ID != "real" {
		t.Errorf("comments after Do = %v, want GitHub's reply in place of the placeholder", threadBodies(th))
	}
}

func TestReplyWithoutAPendingReviewPostsAtOnce(t *testing.T) {
	api := &fakeAPI{}
	api.reply = core.ThreadComment{ID: "real"}
	s := opService(t, api)
	op := s.Reply(repo, 1, "t2", "hi", "octocat")
	if th, _ := opThread(t, s, "t2"); th.Comments[1].Pending {
		t.Error("placeholder is pending, want it to post at once")
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := api.callsOf(); len(got) != 1 || got[0].args[1] != "" {
		t.Errorf("calls = %+v, want one reply with no review", got)
	}
}

func TestReplyRefusedForAPendingReviewReadsItAndRetries(t *testing.T) {
	api := &fakeAPI{}
	api.reply = core.ThreadComment{ID: "real"}
	api.replyErrs = []error{github.ErrPendingReview}
	s := opService(t, api)
	// GitHub has one the first read didn't see.
	if _, _, err := s.PendingReview(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	api.pendingReview, api.pendingFound = core.PendingReview{ID: "PRR_2"}, true

	if err := s.Reply(repo, 1, "t2", "hi", "octocat").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	got := api.callsOf()
	if len(got) != 2 || got[0].args[1] != "" || got[1].args[1] != "PRR_2" {
		t.Errorf("calls = %+v, want a reply with no review, then one into PRR_2", got)
	}
}

func TestReplyFailureTakesThePlaceholderOut(t *testing.T) {
	api := &fakeAPI{}
	api.opsErr = map[string]error{"ReplyToReviewThread": errors.New("boom")}
	s := opService(t, api)
	op := s.Reply(repo, 1, "t2", "hi", "octocat")
	err := op.Do(t.Context())
	if err == nil || !strings.Contains(err.Error(), "reply on pull eggzec/gh-tui#1") {
		t.Fatalf("Do = %v, want the wrapped error", err)
	}
	if th, _ := opThread(t, s, "t2"); len(th.Comments) != 1 {
		t.Errorf("comments = %v, want the placeholder gone", threadBodies(th))
	}
}

func TestEditPullKeepsTheNewBodyAfterReconcile(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	api := &fakeAPI{}
	api.pullAt = at
	s := opService(t, api)

	op := s.EditPull(repo, 1, "New", "new body")
	d, _ := s.CachedGet(repo, 1)
	if d.Title != "New" || d.Body != "new body" || d.BodyEditedAt.IsZero() {
		t.Fatalf("detail before Do = %q, %q, %v", d.Title, d.Body, d.BodyEditedAt)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	d, _ = s.CachedGet(repo, 1)
	if d.Title != "New" || d.Body != "new body" || !d.BodyEditedAt.Equal(at) {
		t.Errorf("detail after Do = %q, %q, %v, want the body GitHub returned", d.Title, d.Body, d.BodyEditedAt)
	}
}

func TestEditPullRollsBack(t *testing.T) {
	api := &fakeAPI{}
	api.opsErr = map[string]error{"UpdatePullRequestText": errors.New("boom")}
	s := opService(t, api)
	err := s.EditPull(repo, 1, "New", "new body").Do(t.Context())
	if err == nil || !strings.Contains(err.Error(), "edit pull eggzec/gh-tui#1") {
		t.Fatalf("Do = %v, want the wrapped error", err)
	}
	d, _ := s.CachedGet(repo, 1)
	if d.Title != "Old" || d.Body != "old body" || !d.BodyEditedAt.IsZero() {
		t.Errorf("detail after the failure = %q, %q, %v, want it rolled back", d.Title, d.Body, d.BodyEditedAt)
	}
}

func TestEditPullRefusedWhenTheViewerMayNotUpdate(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	s.details.Mutate(detailKey(repo, 1), func(d core.PullRequestDetail) core.PullRequestDetail {
		d.Caps = core.ItemCaps{Known: true}
		return d
	})
	err := s.EditPull(repo, 1, "New", "b").Do(t.Context())
	if !errors.Is(err, errCannotEdit) {
		t.Fatalf("Do = %v, want errCannotEdit", err)
	}
	if d, _ := s.CachedGet(repo, 1); d.Title != "Old" || len(api.callsOf()) != 0 {
		t.Errorf("title = %q, calls = %v, want nothing changed or sent", d.Title, api.callsOf())
	}
}

func TestCommentOpsRefusedByTheTokenChangeNothing(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	s.access = tokenWith(classic("read:org"), true)
	ops := map[string]func() error{
		"EditPull":          func() error { return s.EditPull(repo, 1, "N", "b").Do(t.Context()) },
		"Reply":             func() error { return s.Reply(repo, 1, "t2", "b", "me").Do(t.Context()) },
		"EditComment":       func() error { return s.EditComment(repo, 1, "c1", "b").Do(t.Context()) },
		"EditReviewComment": func() error { return s.EditReviewComment(repo, 1, "t1", "t1a", "b").Do(t.Context()) },
		"EditReviewBody":    func() error { return s.EditReviewBody(repo, 1, "r1", "b").Do(t.Context()) },
		"DeleteComment":     func() error { return s.DeleteComment(repo, 1, "c1").Do(t.Context()) },
		"DeleteReviewComm":  func() error { return s.DeleteReviewComment(repo, 1, "t1", "t1a").Do(t.Context()) },
	}
	for name, do := range ops {
		if err := do(); err == nil {
			t.Errorf("%s succeeded, want the token's refusal", name)
		}
	}
	if got := commentIDs(t, s, firstPage); !slices.Equal(got, []string{"c1", "c2"}) {
		t.Errorf("comments = %q, want them unchanged", got)
	}
	if th, _ := opThread(t, s, "t1"); len(th.Comments) != 2 || len(opThreadsLen(s)) != 2 {
		t.Errorf("threads changed: %v", threadBodies(th))
	}
	if d, _ := s.CachedGet(repo, 1); d.Title != "Old" {
		t.Errorf("title = %q, want Old", d.Title)
	}
	if n := len(api.callsOf()); n != 0 {
		t.Errorf("%d calls were sent, want none", n)
	}
}

func opThreadsLen(s *Service) []core.Thread {
	for _, e := range s.threads.TaggedEntries(detailKey(repo, 1)) {
		return e.Value.Threads
	}
	return nil
}

func TestUnconfirmedCommentsAreRefused(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	if err := s.EditComment(repo, 1, pendingPrefix+"1", "b").Do(t.Context()); err == nil {
		t.Error("EditComment of a placeholder succeeded, want a refusal")
	}
	if err := s.DeleteReviewComment(repo, 1, "t1", pendingPrefix+"1").Do(t.Context()); err == nil {
		t.Error("DeleteReviewComment of a placeholder succeeded, want a refusal")
	}
	if len(api.callsOf()) != 0 {
		t.Error("a placeholder was sent")
	}
}

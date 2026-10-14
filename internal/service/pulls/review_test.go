package pulls

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/tray"
)

var reviewDrafts = []tray.Draft{
	{ID: "d1", Commit: "abc123", Path: "a.go", Side: tray.Right, Line: 7, Body: "one line"},
	{ID: "d2", Commit: "abc123", Path: "b.go", Side: tray.Left, Line: 12, StartLine: 9, Body: "a range"},
	{ID: "d3", Commit: "abc123", ThreadID: "t1", Body: "a reply"},
	// A draft of an earlier version of the tray names no commit.
	{ID: "d4", Path: "c.go", Side: tray.Right, Line: 1, Body: "legacy"},
}

// reviewReads is a service that has read everything a review changes: the
// detail, the threads, the reviews and the comments, each once.
func reviewReads(t *testing.T, api *fakeAPI, opts ...Option) *Service {
	t.Helper()
	r := &threadServer{}
	api.threads = r.serve
	api.get = detailWithThreads
	api.reviews = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Review], error) {
		return core.Page[core.Review]{Items: []core.Review{{ID: "r1"}}}, nil
	}
	api.comments = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Comment], error) {
		return core.Page[core.Comment]{Items: []core.Comment{{ID: "c1"}}}, nil
	}
	s := New(api, opts...)
	readThreads(t, s, threadsQ)
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reviews(t.Context(), ReviewsQuery{Repo: repo, Number: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Comments(t.Context(), CommentsQuery{Repo: repo, Number: 1}); err != nil {
		t.Fatal(err)
	}
	return s
}

// readAgain reads what reviewReads read, and returns how many requests that
// took of each kind.
func readAgain(t *testing.T, s *Service, api *fakeAPI) map[string]int {
	t.Helper()
	before := api.callsSoFar()
	readThreads(t, s, threadsQ)
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reviews(t.Context(), ReviewsQuery{Repo: repo, Number: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Comments(t.Context(), CommentsQuery{Repo: repo, Number: 1}); err != nil {
		t.Fatal(err)
	}
	made := make(map[string]int)
	for k, n := range api.callsSoFar() {
		made[k] = n - before[k]
	}
	return made
}

func TestSubmitReviewSendsAndRefreshesCaches(t *testing.T) {
	api := &fakeAPI{}
	store := openStore(t)
	s := reviewReads(t, api, WithStore(store))

	err := s.SubmitReview(t.Context(), repo, 1, "abc123", core.ReviewEventRequestChanges, "please fix", reviewDrafts)
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}

	want := submitted{
		pull: "PR_1", commit: "abc123", body: "please fix", event: core.ReviewEventRequestChanges,
		threads: []github.ReviewThreadDraft{
			{Path: "a.go", Side: "RIGHT", Line: 7, Body: "one line"},
			{Path: "b.go", Side: "LEFT", Line: 12, StartLine: 9, Body: "a range"},
			{Path: "c.go", Side: "RIGHT", Line: 1, Body: "legacy"},
		},
		replies: []github.ReviewReplyDraft{{ThreadID: "t1", Body: "a reply"}},
	}
	if len(api.submits) != 1 || !reflect.DeepEqual(api.submits[0], want) {
		t.Fatalf("sent %+v, want one review %+v", api.submits, want)
	}
	if n := api.count("id"); n != 0 {
		t.Errorf("asked for the node ID %d times, though the detail has it", n)
	}

	// The next read of each goes to GitHub, though they were fresh.
	made := readAgain(t, s, api)
	for _, k := range []string{"threads", "get", "reviews", "comments"} {
		if made[k] != 1 {
			t.Errorf("%s requests after the review = %d, want 1; all: %v", k, made[k], made)
		}
	}
}

func TestSubmitReviewDropsWhatWasKept(t *testing.T) {
	api := &fakeAPI{}
	store := openStore(t)
	s := reviewReads(t, api, WithStore(store))
	if err := s.SubmitReview(t.Context(), repo, 1, "abc123", core.ReviewEventComment, "", nil); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}

	// The next session reads everything from GitHub, though what it kept was
	// read within the TTL.
	api2 := &fakeAPI{}
	reviewReads(t, api2, WithStore(store))
	for _, k := range []string{"threads", "get", "reviews", "comments"} {
		if n := api2.count(k); n != 1 {
			t.Errorf("%s requests of a new session = %d, want 1: nothing kept", k, n)
		}
	}
}

func TestSubmitReviewFailureLeavesCaches(t *testing.T) {
	api := &fakeAPI{submitErr: errors.New("nope")}
	s := reviewReads(t, api)

	err := s.SubmitReview(t.Context(), repo, 1, "abc123", core.ReviewEventComment, "", reviewDrafts)
	if !errors.Is(err, api.submitErr) {
		t.Fatalf("SubmitReview = %v, want the failure", err)
	}
	if made := readAgain(t, s, api); made["threads"]+made["get"]+made["reviews"]+made["comments"] != 0 {
		t.Errorf("requests after a failed review = %v, want none: the caches were fresh", made)
	}
}

func TestSubmitReviewPendingReviewIsKnown(t *testing.T) {
	api := &fakeAPI{submitErr: github.ErrPendingReview}
	s := reviewReads(t, api)
	err := s.SubmitReview(t.Context(), repo, 1, "abc123", core.ReviewEventComment, "", nil)
	if !errors.Is(err, ErrPendingReview) {
		t.Errorf("SubmitReview = %v, want an error that wraps ErrPendingReview", err)
	}
}

func TestSubmitReviewRefusedSendsNothing(t *testing.T) {
	api := &fakeAPI{}
	s := reviewReads(t, api)
	s.access, s.repos = tokenWith(classic("gist"), true), private
	sent := api.callsSoFar()

	err := s.SubmitReview(t.Context(), repo, 1, "abc123", core.ReviewEventComment, "", reviewDrafts)
	if _, ok := errors.AsType[*core.ScopeError](err); !ok {
		t.Fatalf("SubmitReview = %v, want a ScopeError", err)
	}
	if got := api.callsSoFar(); !reflect.DeepEqual(got, sent) {
		t.Errorf("calls = %v, want %v: nothing sent", got, sent)
	}
	if made := readAgain(t, s, api); made["threads"]+made["get"]+made["reviews"]+made["comments"] != 0 {
		t.Errorf("requests after a refused review = %v, want none", made)
	}
}

func TestSubmitReviewRefusesWhatItCannotSendAsOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		head   string
		event  core.ReviewEvent
		drafts []tray.Draft
		want   error
	}{
		{"older commit", "def456", core.ReviewEventComment, reviewDrafts, errOutdatedDraft},
		{"no head", "", core.ReviewEventComment, nil, errNoHead},
		{"unknown event", "abc123", "PENDING", nil, errReviewEvent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{}
			s := reviewReads(t, api)
			if err := s.SubmitReview(t.Context(), repo, 1, tc.head, tc.event, "", tc.drafts); !errors.Is(err, tc.want) {
				t.Errorf("SubmitReview = %v, want %v", err, tc.want)
			}
			if n := api.count("submit"); n != 0 {
				t.Errorf("submits = %d, want none", n)
			}
		})
	}
}

func TestSubmitReviewAsksForTheNodeID(t *testing.T) {
	api := &fakeAPI{id: func(context.Context, core.RepoRef, int) (string, error) { return "PR_9", nil }}
	s := New(api)
	if err := s.SubmitReview(t.Context(), repo, 9, "abc123", core.ReviewEventApprove, "", nil); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if len(api.submits) != 1 || api.submits[0].pull != "PR_9" {
		t.Errorf("sent %+v, want one review of PR_9", api.submits)
	}

	api = &fakeAPI{id: func(context.Context, core.RepoRef, int) (string, error) { return "", errors.New("gone") }}
	if err := New(api).SubmitReview(t.Context(), repo, 9, "abc123", core.ReviewEventApprove, "", nil); err == nil {
		t.Error("SubmitReview without a node ID = nil, want the failure")
	}
	if api.count("submit") != 0 {
		t.Error("a review was sent without a pull request to send it to")
	}
}

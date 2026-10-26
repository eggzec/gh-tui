package pulls

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func TestSendReviewStartsAReview(t *testing.T) {
	api := &fakeAPI{}
	s := reviewReads(t, api)

	if err := s.SendReview(t.Context(), repo, 1, "abc123", core.ReviewEventApprove, "looks good"); err != nil {
		t.Fatalf("SendReview: %v", err)
	}
	want := []submitted{{pull: "PR_1", commit: "abc123", body: "looks good", event: core.ReviewEventApprove}}
	if !reflect.DeepEqual(api.submits, want) || api.count("submit") != 1 || api.count("submitpending") != 0 {
		t.Fatalf("sent %+v, want one new review %+v", api.submits, want)
	}
	made := readAgain(t, s, api)
	for _, k := range []string{"threads", "get", "reviews", "comments"} {
		if made[k] != 1 {
			t.Errorf("%s requests after the review = %d, want 1; all: %v", k, made[k], made)
		}
	}
}

func TestSendReviewSubmitsThePendingReview(t *testing.T) {
	api := &fakeAPI{pendingReview: pendingOnHead(), pendingFound: true}
	s := reviewReads(t, api)

	if err := s.SendReview(t.Context(), repo, 1, "abc123", core.ReviewEventRequestChanges, "please fix"); err != nil {
		t.Fatalf("SendReview: %v", err)
	}
	// The review's ID stands where a new review has the commit.
	want := []submitted{{pull: "PR_1", commit: "PRR_pending", body: "please fix", event: core.ReviewEventRequestChanges}}
	if !reflect.DeepEqual(api.submits, want) || api.count("submit") != 0 {
		t.Fatalf("sent %+v, want the pending review submitted %+v", api.submits, want)
	}
}

func TestSendReviewUsesAPendingReviewThatAppeared(t *testing.T) {
	api := &fakeAPI{submitErr: github.ErrPendingReview}
	s := reviewReads(t, api)
	// The first read finds none; one is started on the web before the send.
	if _, ok, err := s.PendingReview(t.Context(), repo, 1); err != nil || ok {
		t.Fatalf("PendingReview = %v, %v; want none", ok, err)
	}
	api.pendingReview, api.pendingFound = pendingOnHead(), true

	err := s.SendReview(t.Context(), repo, 1, "abc123", core.ReviewEventComment, "hello")
	if err == nil {
		t.Fatal("SendReview = nil, want the failure of the fake")
	}
	if api.count("submit") != 1 || api.count("submitpending") != 1 {
		t.Fatalf("calls = new %d, pending %d; want one of each", api.count("submit"), api.count("submitpending"))
	}
	if got := api.submits[1]; got.commit != "PRR_pending" || got.body != "hello" {
		t.Errorf("second submission = %+v, want the pending review with the summary", got)
	}
}

func TestSendReviewDoesNotRetryAnEmptyApproval(t *testing.T) {
	api := &fakeAPI{submitErr: github.ErrPendingReview}
	s := reviewReads(t, api)
	if _, ok, err := s.PendingReview(t.Context(), repo, 1); err != nil || ok {
		t.Fatalf("PendingReview = %v, %v; want none", ok, err)
	}
	api.pendingReview, api.pendingFound = pendingOnHead(), true

	err := s.SendReview(t.Context(), repo, 1, "abc123", core.ReviewEventApprove, "")
	if !errors.Is(err, ErrPendingAppeared) {
		t.Fatalf("SendReview = %v, want ErrPendingAppeared", err)
	}
	if api.count("submitpending") != 0 {
		t.Errorf("pending review submitted %d times, want 0", api.count("submitpending"))
	}
}

func TestSendReviewFailures(t *testing.T) {
	boom := errors.New("nope")
	t.Run("send", func(t *testing.T) {
		api := &fakeAPI{submitErr: boom}
		s := reviewReads(t, api)
		if err := s.SendReview(t.Context(), repo, 1, "abc123", core.ReviewEventComment, "x"); !errors.Is(err, boom) {
			t.Errorf("SendReview = %v, want the failure", err)
		}
	})
	t.Run("read", func(t *testing.T) {
		api := &fakeAPI{pendingErr: boom}
		s := reviewReads(t, api)
		if err := s.SendReview(t.Context(), repo, 1, "abc123", core.ReviewEventComment, "x"); !errors.Is(err, boom) {
			t.Errorf("SendReview = %v, want the failure", err)
		}
		if len(api.submits) != 0 {
			t.Errorf("sent %+v, want nothing: the pending review is unknown", api.submits)
		}
	})
	t.Run("outdated", func(t *testing.T) {
		api := &fakeAPI{pendingReview: pendingOnHead(), pendingFound: true}
		s := reviewReads(t, api)
		if err := s.SendReview(t.Context(), repo, 1, "def456", core.ReviewEventComment, "x"); !errors.Is(err, ErrPendingReviewOutdated) {
			t.Errorf("SendReview = %v, want ErrPendingReviewOutdated", err)
		}
	})
}

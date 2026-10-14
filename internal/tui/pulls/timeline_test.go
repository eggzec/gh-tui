package pulls

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

func at(h int) time.Time { return clock.Add(time.Duration(h) * time.Hour) }

func comment(id string, h int) core.Comment {
	return core.Comment{ID: id, Author: core.User{Login: "commenter-" + id}, Body: "body of " + id, CreatedAt: at(h)}
}

func submitted(id string, state core.ReviewState, body string, h int) core.Review {
	return core.Review{ID: id, Author: core.User{Login: "reviewer-" + id}, State: state, Body: body, SubmittedAt: at(h)}
}

func ids(es []entry) []string {
	var out []string
	for _, e := range es {
		if e.isReview {
			out = append(out, "r:"+e.review.ID)
		} else {
			out = append(out, "c:"+e.comment.ID)
		}
	}
	return out
}

func TestTimelineMergesInTimeOrder(t *testing.T) {
	t.Parallel()
	comments := []core.Comment{comment("a", 1), comment("b", 3), comment("c", 5)}
	reviews := []core.Review{
		submitted("late", core.ReviewStateApproved, "", 9),
		submitted("pending", core.ReviewStatePending, "", 2),
		submitted("mid", core.ReviewStateCommented, "", 4),
		submitted("early", core.ReviewStateChangesRequested, "", 0),
	}
	got := strings.Join(ids(timeline(comments, false, reviews, time.Time{})), " ")
	if want := "r:early c:a c:b r:mid c:c r:late"; got != want {
		t.Errorf("the timeline is %s, want %s", got, want)
	}
}

// A page that more follow takes the reviews up to its last comment, and the
// page after it the rest, each review once.
func TestTimelinePagesShareTheReviews(t *testing.T) {
	t.Parallel()
	reviews := []core.Review{submitted("x", core.ReviewStateApproved, "", 2), submitted("y", core.ReviewStateApproved, "", 6)}
	first := timeline([]core.Comment{comment("a", 1), comment("b", 3)}, true, reviews, time.Time{})
	if got, want := strings.Join(ids(first), " "), "c:a r:x c:b"; got != want {
		t.Errorf("the first page is %s, want %s", got, want)
	}
	cursor := joinCursor("2", lastAt([]core.Comment{comment("a", 1), comment("b", 3)}, time.Time{}))
	inner, after := splitCursor(cursor)
	if inner != "2" || !after.Equal(at(3)) {
		t.Fatalf("the cursor split into %q and %v", inner, after)
	}
	second := timeline([]core.Comment{comment("c", 5)}, false, reviews, after)
	if got, want := strings.Join(ids(second), " "), "c:c r:y"; got != want {
		t.Errorf("the second page is %s, want %s", got, want)
	}
	if joinCursor("", at(3)) != "" {
		t.Error("the last page has a cursor")
	}
}

// view returns the text of the conversation.
func conversation(t *testing.T, svc *fakeService) (*host, *detailModal) {
	t.Helper()
	svc.thread = []core.Comment{comment("a", -5), comment("z", 5)}
	svc.threadPages = map[string]core.ThreadPage{"": {
		Threads: []core.Thread{
			{ID: "t1", Path: "a.go", Line: 1, ReviewID: "R1", Comments: []core.ThreadComment{{ID: "c1", Body: "x"}}, TotalComments: 1},
			{ID: "t2", Path: "b.go", Line: 2, ReviewID: "R1", Resolved: true, Comments: []core.ThreadComment{{ID: "c2", Body: "y"}}, TotalComments: 1},
			{ID: "t3", Path: "c.go", Line: 3, ReviewID: "R2", Comments: []core.ThreadComment{{ID: "c3", Body: "z"}}, TotalComments: 1},
		},
		Total: 4, Next: "p2",
	}, "p2": {
		Threads: []core.Thread{{ID: "t4", Path: "d.go", Line: 4, ReviewID: "R1", Comments: []core.ThreadComment{{ID: "c4", Body: "w"}}, TotalComments: 1}},
		Total:   4,
	}}
	return threaded(t, svc, 116, 60, "unicode")
}

func TestConversationShowsReviews(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.reviews = []core.Review{
		submitted("R1", core.ReviewStateChangesRequested, "Please **fix** the cache.", -2),
		submitted("R2", core.ReviewStateApproved, "", -1),
		submitted("R3", core.ReviewStateDismissed, "", 0),
		submitted("R4", core.ReviewStateCommented, "", 1),
		submitted("R5", core.ReviewStatePending, "secret draft", 2),
	}
	_, m := conversation(t, svc)
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"reviewer-R1", "changes requested", "Please fix the cache.", "2 threads, 1 unresolved",
		"reviewer-R2 ✓ approved", "1 thread, 1 unresolved",
		"reviewer-R3 dismissed", "reviewer-R4 commented",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("the conversation lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "secret draft") || strings.Contains(v, "reviewer-R5") {
		t.Errorf("a pending review shows:\n%s", v)
	}
	// R3 and R4 have no body and no threads: a line each, and no summary.
	if strings.Count(v, "0 threads") != 0 {
		t.Errorf("a review with no threads has a summary:\n%s", v)
	}
	order := []string{"commenter-a", "reviewer-R1", "reviewer-R2", "reviewer-R3", "reviewer-R4", "commenter-z"}
	last := -1
	for _, name := range order {
		i := strings.Index(v, name)
		if i < last {
			t.Errorf("%s is out of order in:\n%s", name, v)
		}
		last = max(last, i)
	}
}

// The counts follow the threads as more pages load and as a thread is
// resolved.
func TestReviewCountsFollowTheThreads(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.reviews = []core.Review{submitted("R1", core.ReviewStateCommented, "", -2)}
	h, m := conversation(t, svc)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "2 threads, 1 unresolved") {
		t.Fatalf("the first page's count is missing:\n%s", v)
	}
	drain(t, h, m.readMoreThreads())
	if v := ansi.Strip(m.View()); !strings.Contains(v, "3 threads, 2 unresolved") {
		t.Fatalf("the count didn't take the next page:\n%s", v)
	}
	drain(t, h, m.resolved(threadResolvedMsg{thread: m.thread.ID(), id: "t1", resolved: true}))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "3 threads, 1 unresolved") {
		t.Fatalf("the count didn't follow the resolve:\n%s", v)
	}
}

// A review that couldn't be read fails the page, which reads again.
func TestReviewsFailedRead(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.reviewsErr = errors.New("boom")
	if _, _, err := fetchTimeline(svc, repo, 142)(t.Context(), ""); err == nil {
		t.Error("a failed read of the reviews passed")
	}
}

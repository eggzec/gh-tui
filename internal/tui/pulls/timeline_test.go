package pulls

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

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
	for i := range es {
		if es[i].isReview {
			out = append(out, "r:"+es[i].review.ID)
		} else {
			out = append(out, "c:"+es[i].comment.ID)
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

// conversation returns the modal on its Conversation tab, 116 columns wide,
// with the comments and the threads that the tests share.
func conversation(t *testing.T, svc *fakeService) (*host, *detailModal) {
	t.Helper()
	return conversationAt(t, svc, 116, 60)
}

// conversationAt is conversation at width by height.
func conversationAt(t *testing.T, svc *fakeService, width, height int) (*host, *detailModal) {
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
	return threaded(t, svc, width, height, "unicode")
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

// A review that couldn't be read leaves the comments, with a line that says
// so at the head of the first page only.
func TestReviewsFailedRead(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.reviewsErr = errors.New("boom")
	got, _, err := fetchTimeline(svc, repo, 142, nil)(t.Context(), "")
	if err != nil {
		t.Fatalf("a failed read of the reviews failed the page: %v", err)
	}
	if len(got) < 2 || got[0].failure == nil {
		t.Errorf("the page has no line for the failed reviews: %v", ids(got))
	}
	got, _, err = fetchTimeline(svc, repo, 142, nil)(t.Context(), joinCursor("2", at(0)))
	if err != nil {
		t.Fatalf("a failed read of the reviews failed the second page: %v", err)
	}
	for _, e := range got {
		if e.failure != nil {
			t.Errorf("the second page has the line for the failed reviews: %v", ids(got))
		}
	}
}

// Paged reviews and paged comments come out as one conversation: each review
// once, in order, with a comment first at the same instant, and a review at
// the last comment's instant on the page of that comment.
func TestFetchTimelinePages(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.commentPages = map[string]core.Page[core.Comment]{
		"":  {Items: []core.Comment{comment("a", 1), comment("b", 3)}, Next: "2"},
		"2": {Items: []core.Comment{comment("c", 5)}},
	}
	svc.reviewPages = map[string]core.Page[core.Review]{
		"":   {Items: []core.Review{submitted("x", core.ReviewStateApproved, "", 2), submitted("w", core.ReviewStateCommented, "", 3)}, Next: "r2"},
		"r2": {Items: []core.Review{submitted("y", core.ReviewStateApproved, "", 4), submitted("v", core.ReviewStateCommented, "", 5), submitted("z", core.ReviewStateApproved, "", 9)}},
	}
	fetch := fetchTimeline(svc, repo, 142, nil)
	first, next, err := fetch(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids(first), " "), "c:a r:x c:b r:w"; got != want {
		t.Errorf("the first page is %s, want %s", got, want)
	}
	if next == "" {
		t.Fatal("the first page has no cursor")
	}
	second, last, err := fetch(t.Context(), next)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ids(second), " "), "r:y c:c r:v r:z"; got != want {
		t.Errorf("the second page is %s, want %s", got, want)
	}
	if last != "" {
		t.Errorf("the last page has the cursor %q", last)
	}
	if got, want := strings.Join(svc.reviewReads, ","), ",r2,,r2"; got != want {
		t.Errorf("the reviews were read at the cursors %q, want %q", got, want)
	}
}

// The cursor holds the comments' cursor whole, even with the separator in it.
func TestCursorKeepsTheCommentsCursor(t *testing.T) {
	t.Parallel()
	inner, after := splitCursor(joinCursor("https://u@host/p?page=2", at(3)))
	if inner != "https://u@host/p?page=2" || !after.Equal(at(3)) {
		t.Errorf("the cursor split into %q and %v", inner, after)
	}
}

// The first page from memory has the reviews if they are cached, and the
// comments alone if they aren't.
func TestCachedFirstWithAndWithoutReviews(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.commentPages = map[string]core.Page[core.Comment]{"": {Items: []core.Comment{comment("a", 1), comment("b", 3)}}}
	svc.reviews = []core.Review{submitted("x", core.ReviewStateApproved, "", 2)}
	svc.commented[commentsQuery(repo, 142)] = true
	got, _, ok := cachedFirst(svc, repo, 142)
	if !ok || strings.Join(ids(got), " ") != "c:a c:b" {
		t.Errorf("without cached reviews the page is %v, %v", ids(got), ok)
	}
	svc.reviewed = true
	got, _, ok = cachedFirst(svc, repo, 142)
	if !ok || strings.Join(ids(got), " ") != "c:a r:x c:b" {
		t.Errorf("with cached reviews the page is %v, %v", ids(got), ok)
	}
	delete(svc.commented, commentsQuery(repo, 142))
	if _, _, ok = cachedFirst(svc, repo, 142); ok {
		t.Error("a page without cached comments is cached")
	}
}

// The conversation fits the width it is given, 80 columns and narrower, with
// each verdict, a body, a review with nothing to say, a thread summary and a
// login too long for the line.
func TestConversationFitsNarrowWidths(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
	}{{"80x24", 76, 40}, {"60x24", 56, 40}} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			long := submitted("R4", core.ReviewStateCommented, "", 1)
			long.Author.Login = "a-reviewer-with-a-very-long-login-name-indeed"
			svc.reviews = []core.Review{
				submitted("R1", core.ReviewStateChangesRequested, "Please **fix** the cache, it loses a page when two writers race on the same key.", -2),
				submitted("R2", core.ReviewStateApproved, "", -1),
				submitted("R3", core.ReviewStateDismissed, "", 0),
				long,
			}
			_, m := conversationAt(t, svc, tt.width, tt.height)
			v := m.View()
			for i, l := range strings.Split(v, "\n") {
				if w := ansi.StringWidth(l); w != tt.width {
					t.Errorf("line %d is %d cells wide, want %d", i+1, w, tt.width)
				}
			}
			golden.RequireEqual(t, v)
		})
	}
}

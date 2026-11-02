package issues

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func TestEditComment(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	at := epoch.Add(time.Hour)
	api.editComment = func(_, body string) (core.Edited, error) {
		return core.Edited{Body: body + " (server)", At: at}, nil
	}

	op := s.EditComment(repo, 7, "IC_2", "New")
	p, _ := s.CachedComments(firstOfTwo)
	if got := p.Items[1].Body; got != "New" {
		t.Errorf("body before sending = %q, want New", got)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	p, _ = s.CachedComments(firstOfTwo)
	if c := p.Items[1]; c.Body != "New (server)" || !c.UpdatedAt.Equal(at) {
		t.Errorf("comment = %q at %v, want GitHub's body and time", c.Body, c.UpdatedAt)
	}
	api.checkCalls(t, "EditIssueComment")
}

func TestEditCommentRollsBack(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.editComment = func(string, string) (core.Edited, error) { return core.Edited{}, fakeError("boom") }

	if err := s.EditComment(repo, 7, "IC_2", "New").Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	p, _ := s.CachedComments(firstOfTwo)
	if got := p.Items[1].Body; got != "Comment 2" {
		t.Errorf("body = %q, want the old one back", got)
	}
}

func TestEditCommentRefusesPending(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	if err := s.EditComment(repo, 7, "pending:1", "x").Do(t.Context()); !errors.Is(err, errUnsent) {
		t.Errorf("edit err = %v, want errUnsent", err)
	}
	if err := s.DeleteComment(repo, 7, "pending:1").Do(t.Context()); !errors.Is(err, errUnsent) {
		t.Errorf("delete err = %v, want errUnsent", err)
	}
	api.checkCalls(t)
}

func TestDeleteComment(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.deleteComment = func(string) error { return nil }

	op := s.DeleteComment(repo, 7, "IC_3")
	checkComments(t, s, "after delete", map[CommentsQuery][]string{
		lastOfTwo:   nil,
		onlyOfThree: {"IC_1", "IC_2"},
	})
	checkIssue7(t, s, "no comments counted", func(it core.Issue) bool { return it.Comments == 0 })
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	api.checkCalls(t, "DeleteIssueComment")
}

func TestDeleteCommentRollsBack(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.deleteComment = func(string) error { return fakeError("boom") }

	if err := s.DeleteComment(repo, 7, "IC_3").Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	checkComments(t, s, "after rollback", nil)
	checkIssue7(t, s, "the count back", func(it core.Issue) bool { return it.Comments == 1 })
}

func TestEditLabels(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	var added, removed []string
	api.addLabels = func(_ int, names []string) ([]core.Label, error) {
		added = names
		return []core.Label{{Name: "bug"}, enhancement}, nil
	}
	api.removeLabel = func(_ int, name string) ([]core.Label, error) {
		removed = append(removed, name)
		if name == "gone" {
			return nil, core.ErrNotFound
		}
		return []core.Label{enhancement}, nil
	}

	op := s.EditLabels(repo, 7, []core.Label{enhancement}, []string{"BUG", "gone"})
	checkIssue7(t, s, "only enhancement", func(it core.Issue) bool {
		return len(it.Labels) == 1 && it.Labels[0] == enhancement
	})
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !slices.Equal(added, []string{"enhancement"}) || !slices.Equal(removed, []string{"BUG", "gone"}) {
		t.Errorf("sent add %q, remove %q", added, removed)
	}
	checkIssue7(t, s, "only enhancement", func(it core.Issue) bool {
		return len(it.Labels) == 1 && it.Labels[0] == enhancement
	})
}

func TestEditLabelsRollsBack(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.addLabels = func(int, []string) ([]core.Label, error) { return nil, fakeError("boom") }

	if err := s.EditLabels(repo, 7, []core.Label{enhancement}, nil).Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	checkIssue7(t, s, "bug only", func(it core.Issue) bool { return len(it.Labels) == 1 && it.Labels[0].Name == "bug" })
}

func TestEditAssignees(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	hubot := core.User{Login: "hubot"}
	api.addAssignees = func(number int, _ []string) (core.Issue, error) {
		it := issue(number)
		it.Assignees = []core.User{{Login: "hubot", Name: "From GitHub"}}
		return it, nil
	}

	op := s.EditAssignees(repo, 7, []core.User{hubot}, nil)
	checkIssue7(t, s, "hubot assigned", func(it core.Issue) bool { return len(it.Assignees) == 1 })
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	checkIssue7(t, s, "GitHub's assignee", func(it core.Issue) bool {
		return len(it.Assignees) == 1 && it.Assignees[0].Name == "From GitHub"
	})
}

func TestEditAssigneesRollsBack(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.addAssignees = func(int, []string) (core.Issue, error) { return core.Issue{}, fakeError("boom") }

	if err := s.EditAssignees(repo, 7, []core.User{{Login: "hubot"}}, nil).Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	checkIssue7(t, s, "no assignees", func(it core.Issue) bool { return len(it.Assignees) == 0 })
}

// timelineQuery is the page of issue 7's timeline that primeTimeline caches.
var timelineQuery = TimelineQuery{Repo: repo, Number: 7, PageSize: 50}

// primeTimeline caches a timeline page of issue 7 that holds comment IC_2.
func primeTimeline(t *testing.T, s *Service, api *fakeAPI) {
	t.Helper()
	api.timeline = func(int, string, int) (core.Page[core.TimelineItem], error) {
		return core.Page[core.TimelineItem]{Items: []core.TimelineItem{
			{ID: "IC_1", Body: "Comment 1"},
			{ID: "IC_2", Body: "Comment 2"},
		}}, nil
	}
	if _, err := s.Timeline(t.Context(), timelineQuery); err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	api.called()
}

func timelineBodies(t *testing.T, s *Service) ([]string, bool) {
	t.Helper()
	p, ok := s.CachedTimeline(timelineQuery)
	var out []string
	for _, it := range p.Items {
		out = append(out, it.Body)
	}
	return out, ok
}

// checkTimelineForgotten reports an error unless the timeline page goes to
// GitHub again, which it does once it is stale.
func checkTimelineForgotten(t *testing.T, s *Service, api *fakeAPI) {
	t.Helper()
	api.called()
	if _, err := s.Timeline(t.Context(), timelineQuery); err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	api.checkCalls(t, "ListIssueTimeline")
}

// checkRefetch reads issue 7 and its list, and reports an error unless each
// one went to GitHub, which is so when it is stale.
func checkRefetch(t *testing.T, s *Service, api *fakeAPI) {
	t.Helper()
	api.called()
	api.getIssue = func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
		return issue(number), ok(`"i2"`), nil
	}
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
		t.Fatalf("List: %v", err)
	}
	api.checkCalls(t, "GetIssue", "ListIssues")
}

func TestEditIssue(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	at := epoch.Add(time.Hour)
	s.now = func() time.Time { return at }
	got := epoch.Add(2 * time.Hour)
	api.updateText = func(number int, title, body string) (core.Issue, error) {
		it := issue(number)
		it.Title, it.Body, it.UpdatedAt = title+" (server)", body+" (server)", got
		return it, nil
	}

	op := s.EditIssue(repo, 7, "New", "Text")
	checkIssue7(t, s, "the new text at the edit time", func(it core.Issue) bool {
		return it.Title == "New" && it.Body == "Text" && it.BodyEditedAt.Equal(at)
	})
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	api.checkCalls(t, "UpdateIssueText")
	d, _ := s.CachedGet(repo, 7)
	if d.Title != "New (server)" || d.Body != "Text (server)" || !d.BodyEditedAt.Equal(got) {
		t.Errorf("detail = %q %q at %v, want GitHub's text and time", d.Title, d.Body, d.BodyEditedAt)
	}
	// The lists are stale, since the title may move the issue within them.
	if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
		t.Fatalf("List: %v", err)
	}
	api.checkCalls(t, "ListIssues")
}

func TestEditIssueRollsBack(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.updateText = func(int, string, string) (core.Issue, error) { return core.Issue{}, fakeError("boom") }

	if err := s.EditIssue(repo, 7, "New", "Text").Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	checkIssue7(t, s, "the old text", func(it core.Issue) bool {
		return it.Title == "Issue" && it.Body == "" && it.BodyEditedAt.IsZero()
	})
}

func TestEditIssueRefusedByCaps(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.getIssue = func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
		it := issue(number)
		it.Caps = core.ItemCaps{Known: true}
		return it, ok(`"i2"`), nil
	}
	s.Invalidate(repo)
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	api.called()
	if err := s.EditIssue(repo, 7, "New", "Text").Do(t.Context()); !errors.Is(err, errCannotEdit) {
		t.Errorf("err = %v, want errCannotEdit", err)
	}
	api.checkCalls(t)
}

// A change that failed after the add went through is on GitHub, so what the
// cache holds of the issue is stale and its timeline is forgotten.
func TestEditLabelsPartialFailure(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	api.addLabels = func(int, []string) ([]core.Label, error) { return []core.Label{enhancement}, nil }
	api.removeLabel = func(int, string) ([]core.Label, error) { return nil, fakeError("boom") }

	if err := s.EditLabels(repo, 7, []core.Label{enhancement}, []string{"bug"}).Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	checkTimelineForgotten(t, s, api)
	checkRefetch(t, s, api)
}

func TestEditAssigneesPartialFailure(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	api.addAssignees = func(number int, _ []string) (core.Issue, error) { return issue(number), nil }
	api.removeAssignees = func(int, []string) (core.Issue, error) { return core.Issue{}, fakeError("boom") }

	if err := s.EditAssignees(repo, 7, []core.User{{Login: "hubot"}}, []string{"octocat"}).Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	checkTimelineForgotten(t, s, api)
	checkRefetch(t, s, api)
}

// A failure of the first call changed nothing at GitHub, so the rollback is
// enough and the cache stays fresh.
func TestEditLabelsTotalFailureStaysFresh(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.addLabels = func(int, []string) ([]core.Label, error) { return nil, fakeError("boom") }
	_ = s.EditLabels(repo, 7, []core.Label{enhancement}, []string{"bug"}).Do(t.Context())
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	api.checkCalls(t, "AddIssueLabels")
}

func TestEditNothingSendsNothing(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	before := look(t, s)
	if err := s.EditLabels(repo, 7, nil, nil).Do(t.Context()); err != nil {
		t.Errorf("labels: %v", err)
	}
	if err := s.EditAssignees(repo, 7, nil, nil).Do(t.Context()); err != nil {
		t.Errorf("assignees: %v", err)
	}
	api.checkCalls(t)
	if after := look(t, s); !reflect.DeepEqual(after, before) {
		t.Error("an empty edit changed the cache")
	}
}

func TestCommentForgetsTimeline(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	api.comment = func(_ int, body string) (core.Comment, error) {
		c := comment(4)
		c.Body = body
		return c, nil
	}

	if err := s.Comment(repo, 7, "Thanks").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	checkTimelineForgotten(t, s, api)
}

func TestEditCommentOnTimeline(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	api.editComment = func(_, body string) (core.Edited, error) { return core.Edited{Body: body}, nil }

	op := s.EditComment(repo, 7, "IC_2", "New")
	if got, _ := timelineBodies(t, s); !slices.Equal(got, []string{"Comment 1", "New"}) {
		t.Errorf("timeline before sending = %q", got)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, ok := timelineBodies(t, s); !ok {
		t.Error("the timeline page is gone")
	}
}

func TestEditCommentRollsBackTimeline(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	api.editComment = func(string, string) (core.Edited, error) { return core.Edited{}, fakeError("boom") }

	if err := s.EditComment(repo, 7, "IC_2", "New").Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	if got, _ := timelineBodies(t, s); !slices.Equal(got, []string{"Comment 1", "Comment 2"}) {
		t.Errorf("timeline = %q, want the old one back", got)
	}
}

func TestDeleteCommentOnTimeline(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	api.deleteComment = func(string) error { return nil }

	op := s.DeleteComment(repo, 7, "IC_2")
	if got, _ := timelineBodies(t, s); !slices.Equal(got, []string{"Comment 1"}) {
		t.Errorf("timeline before sending = %q", got)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	// The lists count the comment, so they are stale.
	if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
		t.Fatalf("List: %v", err)
	}
	api.checkCalls(t, "DeleteIssueComment", "ListIssues")
}

func TestDeleteCommentRollsBackTimeline(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	api.deleteComment = func(string) error { return fakeError("boom") }

	if err := s.DeleteComment(repo, 7, "IC_2").Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	if got, _ := timelineBodies(t, s); !slices.Equal(got, []string{"Comment 1", "Comment 2"}) {
		t.Errorf("timeline = %q, want the old one back", got)
	}
}

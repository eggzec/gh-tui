package issues

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestEditComment(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	at := epoch.Add(time.Hour)
	api.editComment = func(id, body string) (core.Edited, error) {
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
	api.addAssignees = func(number int, logins []string) (core.Issue, error) {
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

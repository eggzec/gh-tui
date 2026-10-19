package pulls

import (
	"errors"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// keepDrafts stores a draft on the pull request of m, written on commit.
func keepDrafts(t *testing.T, m *panesModal, commits ...string) {
	t.Helper()
	for i, c := range commits {
		d := tray.Draft{ID: string(rune('a' + i)), Commit: c, Path: "a.go", Side: tray.Right, Line: i + 1, Body: "note " + c}
		if _, err := m.draftStore().Put(m.draftKey(), d); err != nil {
			t.Fatal(err)
		}
	}
}

// reviewWithComposer opens the composer of the first verdict, and types text.
func reviewWithComposer(m *panesModal, text string) {
	startReview(m)
	m.Update(keyMsg("enter"))
	typeText(m, text)
}

func TestReviewSendsTheDraftsAndClearsThem(t *testing.T) {
	t.Parallel()
	m, svc := reviewPanes(t)
	keepDrafts(t, m, "abc123", "abc123")
	// An empty summary is allowed: the comments are the review.
	reviewWithComposer(m, "")
	cmd := submit(m)
	if n, ok := find[ui.NotifyMsg](cmd); ok {
		t.Fatalf("a review with comments and no summary was refused: %q", n.Text)
	}
	c, ok := m.top().(*confirmOverlay)
	if !ok {
		t.Fatalf("submit left %T on top, want the question", m.top())
	}
	if q := c.c.Question; !strings.Contains(q, "with 2 comments") {
		t.Errorf("question = %q, want the number of comments", q)
	}
	if len(svc.submits()) != 0 {
		t.Fatal("the review was sent before the answer")
	}

	msg, ok := find[reviewedMsg](m.Update(keyMsg("y")))
	if !ok {
		t.Fatal("yes sent nothing")
	}
	got := svc.submits()
	if len(got) != 1 || got[0].head != "abc123" || got[0].body != "" || len(got[0].drafts) != 2 || got[0].number != m.number {
		t.Fatalf("submitted %+v, want one review of the head with both drafts", got)
	}
	if len(svc.sends()) != 0 {
		t.Error("the review went out through SendReview, which has no comments")
	}
	if left := keptDrafts(m); len(left) != 0 {
		t.Errorf("drafts after the send = %+v, want none", left)
	}
	if n, ok := find[ui.NotifyMsg](m.Update(msg)); !ok || !strings.Contains(n.Text, "Review submitted") {
		t.Errorf("notice %v, %v", n, ok)
	}
}

func TestReviewWithDraftsAndASummary(t *testing.T) {
	t.Parallel()
	m, svc := reviewPanes(t)
	keepDrafts(t, m, "abc123")
	reviewWithComposer(m, "see notes")
	submit(m)
	if _, ok := find[reviewedMsg](m.Update(keyMsg("y"))); !ok {
		t.Fatal("yes sent nothing")
	}
	if got := svc.submits(); len(got) != 1 || got[0].body != "see notes" || len(got[0].drafts) != 1 {
		t.Fatalf("submitted %+v", got)
	}
}

func TestReviewKeepsTheDraftsOnAFailure(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"error":            errors.New("boom"),
		"pending appeared": pulls.ErrPendingAppeared,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, svc := reviewPanes(t)
			svc.submitErr = err
			keepDrafts(t, m, "abc123", "abc123")
			reviewWithComposer(m, "")
			submit(m)
			msg, ok := find[reviewedMsg](m.Update(keyMsg("y")))
			if !ok {
				t.Fatal("yes sent nothing")
			}
			if len(svc.submits()) != 1 {
				t.Fatalf("submits = %d, want one try", len(svc.submits()))
			}
			cmd := m.Update(msg)
			if _, ok := find[ui.NotifyMsg](cmd); !ok && name == "pending appeared" {
				t.Error("no toast for the pending review")
			}
			if got := keptDrafts(m); len(got) != 2 {
				t.Errorf("drafts after a failure = %+v, want both kept", got)
			}
		})
	}
}

func TestReviewWithADraftOnAnOlderCommitIsNotSent(t *testing.T) {
	t.Parallel()
	m, svc := reviewPanes(t)
	keepDrafts(t, m, "abc123", "old")
	reviewWithComposer(m, "")
	cmd := submit(m)
	n, ok := find[ui.NotifyMsg](cmd)
	if !ok || !strings.Contains(n.Text, "Not sent: 1 draft written on an older commit") {
		t.Fatalf("notice %q, %v; want the outdated toast", n.Text, ok)
	}
	if _, ok := m.top().(*composerOverlay); !ok {
		t.Errorf("a refusal left %T on top, want the composer", m.top())
	}
	if len(svc.submits())+len(svc.sends()) != 0 {
		t.Error("a review was sent with a draft on an older commit")
	}
	if got := keptDrafts(m); len(got) != 2 {
		t.Errorf("drafts = %+v, want both kept", got)
	}
}

func TestReviewWithoutDraftsStillNeedsASummary(t *testing.T) {
	t.Parallel()
	m, svc := reviewPanes(t)
	reviewWithComposer(m, "")
	n, ok := find[ui.NotifyMsg](submit(m))
	if !ok || !strings.HasPrefix(n.Text, "Nothing to send") {
		t.Errorf("notice %q, %v", n.Text, ok)
	}
	if len(svc.submits()) != 0 {
		t.Error("a review with nothing in it was sent")
	}
}

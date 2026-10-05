package dashboard

import (
	"cmp"
	"testing"
)

func TestSelected(t *testing.T) {
	s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, 140, 38)
	for _, tt := range []struct {
		pane paneID
		what string
	}{
		{pinnedPane, "repository"},
		{reposPane, "repository"},
		{inboxPane, ""},
	} {
		s.focusPane(tt.pane)
		got, ok := s.Selected()
		if !ok || got.Repo.Owner == "" || got.URL == "" || tt.what != "" && got.What != tt.what {
			t.Errorf("pane %d: Selected() = %+v, %v, want a %s", tt.pane, got, ok, tt.what)
		}
	}
	s.focusPane(workPane)
	if hit, ok := s.tasks.selected(); ok {
		if got, _ := s.Selected(); got.Number != hit.Issue.Number || got.URL != hit.Issue.URL {
			t.Errorf("work: Selected() = %+v, want %+v", got, hit)
		}
	}
	s.focusPane(calendarPane)
	if got, ok := s.Selected(); ok {
		t.Errorf("the calendar selected %+v", got)
	}
}

// TestSelectedOwner checks the owner of each pane's selection: that of the
// repository, the organization of its tab, even an empty one, the author
// of a task, and the repository's owner for a thread of the inbox.
func TestSelectedOwner(t *testing.T) {
	svc := newFake()
	svc.repos["github"] = nil
	s := newSection(t, svc, &fakeInbox{threads: inboxThreads()}, 140, 38)
	s.focusPane(reposPane)
	if got, _ := s.Selected(); got.Owner != "octocat" {
		t.Errorf("yours: owner %q, want octocat", got.Owner)
	}
	press(t, s, "]")
	if got, ok := s.Selected(); !ok || got.Owner != "github" || got.What != "organization" || got.URL != "https://github.com/github" {
		t.Errorf("an empty tab of github: Selected() = %+v, %v, want the organization", got, ok)
	}
	press(t, s, "]")
	if got, _ := s.Selected(); got.Owner != "charmbracelet" || got.Number != 0 || got.Repo.Owner != "charmbracelet" {
		t.Errorf("charmbracelet: Selected() = %+v, want a repository of it", got)
	}
	s.focusPane(pinnedPane)
	if c, ok := s.pinned.Selected(); ok {
		if got, _ := s.Selected(); got.Owner != c.Repo.Ref.Owner {
			t.Errorf("pinned: owner %q, want %q", got.Owner, c.Repo.Ref.Owner)
		}
	}
	s.focusPane(workPane)
	if hit, ok := s.tasks.selected(); ok {
		// A task without an author falls back to its repository's owner.
		if got, _ := s.Selected(); got.Owner != cmp.Or(hit.Issue.Author.Login, hit.Issue.Repo.Owner) || got.Owner == "" {
			t.Errorf("work: owner %q, want that of %+v", got.Owner, hit.Issue)
		}
	}
	s.focusPane(inboxPane)
	if n, ok := s.threads.selected(); ok {
		if got, _ := s.Selected(); got.Owner != n.Repo.Owner {
			t.Errorf("inbox: owner %q, want %q", got.Owner, n.Repo.Owner)
		}
	}
}

package dashboard

import (
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

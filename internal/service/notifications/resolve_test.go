package notifications

import (
	"errors"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func TestListResolvesSubjectURLs(t *testing.T) {
	const list = "https://github.com/o/r/releases"
	items := func() []core.Notification {
		return []core.Notification{
			{ID: "1", Subject: core.Subject{Type: core.SubjectRelease, ReleaseID: 9, URL: "api/releases/9", WebURL: list}},
			{ID: "2", Subject: core.Subject{Type: core.SubjectCheckSuite, CheckSuiteID: 5, URL: "api/check-suites/5", WebURL: "https://github.com/o/r/actions"}},
			{ID: "3", Subject: core.Subject{Type: core.SubjectIssue, Number: 4, WebURL: "https://github.com/o/r/issues/4"}},
		}
	}
	api := &fakeAPI{
		list: func(core.NotificationFilter, int, string, github.Conditional) (page, github.Response, error) {
			return page{Items: items()}, github.Response{LastModified: modified1}, nil
		},
		resolve: func(n core.Notification) (string, error) {
			if n.ID == "2" {
				return "", errors.New("no run")
			}
			return "https://github.com/o/r/releases/tag/v1", nil
		},
	}
	s := New(api)
	p, err := s.List(t.Context(), ListQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := func(i int) core.Subject { return p.Items[i].Subject }
	if s := got(0); s.WebURL != "https://github.com/o/r/releases/tag/v1" || !s.Resolved {
		t.Errorf("release = %+v, want its own page, resolved", s)
	}
	if s := got(1); s.WebURL != "https://github.com/o/r/actions" || s.Resolved {
		t.Errorf("failed check suite = %+v, want the Actions tab, unresolved", s)
	}
	if s := got(2); s.Resolved {
		t.Errorf("issue = %+v, want it left alone", s)
	}
	if n := api.resolves.Load(); n != 2 {
		t.Errorf("resolved %d subjects, want 2", n)
	}
}

func TestResolvedURLLookedUpAgainForUpdatedThread(t *testing.T) {
	// A check suite has no API URL, and GitHub reuses its thread for the next
	// run of the workflow, so only the update time tells the runs apart.
	run := "https://github.com/o/r/actions/runs/100"
	updated := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	api := &fakeAPI{
		list: func(_ core.NotificationFilter, _ int, _ string, cond github.Conditional) (page, github.Response, error) {
			n := core.Notification{ID: "1", UpdatedAt: updated, Subject: core.Subject{Type: core.SubjectCheckSuite, Title: "CI workflow run failed for main branch", WebURL: "actions"}}
			return page{Items: []core.Notification{n}}, github.Response{LastModified: modified1 + cond.LastModified}, nil
		},
		resolve: func(core.Notification) (string, error) { return run, nil },
	}
	s := New(api)
	if _, err := s.List(t.Context(), ListQuery{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	// The same thread, unchanged, keeps its page.
	s.Invalidate()
	if _, err := s.List(t.Context(), ListQuery{}); err != nil {
		t.Fatalf("second List: %v", err)
	}
	if n := api.resolves.Load(); n != 1 {
		t.Fatalf("resolved %d times for an unchanged thread, want 1", n)
	}
	// The next run notifies the same thread again.
	updated = updated.Add(time.Hour)
	run = "https://github.com/o/r/actions/runs/200"
	s.Invalidate()
	p, err := s.List(t.Context(), ListQuery{})
	if err != nil {
		t.Fatalf("third List: %v", err)
	}
	if n := api.resolves.Load(); n != 2 {
		t.Errorf("resolved %d times, want 2: the thread changed", n)
	}
	if sub := p.Items[0].Subject; sub.WebURL != run || !sub.Resolved {
		t.Errorf("subject after an update = %+v, want the new run", sub)
	}
}

func TestResolvedURLKept(t *testing.T) {
	thread := func() core.Notification {
		return core.Notification{ID: "1", Subject: core.Subject{Type: core.SubjectRelease, ReleaseID: 9, URL: "api/releases/9", WebURL: "list"}}
	}
	api := &fakeAPI{
		list: func(_ core.NotificationFilter, _ int, _ string, cond github.Conditional) (page, github.Response, error) {
			// A new answer every time, so each fetch sees fresh threads.
			return page{Items: []core.Notification{thread()}}, github.Response{LastModified: modified1 + cond.LastModified}, nil
		},
		resolve: func(core.Notification) (string, error) { return "own page", nil },
	}
	s := New(api)
	if _, err := s.List(t.Context(), ListQuery{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	s.Invalidate()
	p, err := s.List(t.Context(), ListQuery{})
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if api.lists.Load() != 2 {
		t.Fatalf("listed %d times, want 2", api.lists.Load())
	}
	if sub := p.Items[0].Subject; sub.WebURL != "own page" || !sub.Resolved {
		t.Errorf("subject after a refetch = %+v, want the resolved page", sub)
	}
	if n := api.resolves.Load(); n != 1 {
		t.Errorf("resolved %d times, want 1: the kept URL is not read again", n)
	}
}

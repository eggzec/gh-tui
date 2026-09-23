package notifications

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

const (
	modified1 = "Mon, 21 Sep 2026 10:00:00 GMT"
	modified2 = "Tue, 22 Sep 2026 10:00:00 GMT"
)

var (
	page1 = page{
		Items: []core.Notification{{ID: "1", Subject: core.Subject{Title: "first"}, Unread: true}},
		Next:  "https://api.github.com/notifications?page=2",
	}
	page2 = page{Items: []core.Notification{{ID: "2", Subject: core.Subject{Title: "second"}, Unread: true}}}
)

// servePage1 answers with page1, or with a 304 when the request carries its
// Last-Modified already.
func servePage1(_ core.NotificationFilter, _ string, cond github.Conditional) (page, github.Response, error) {
	res := github.Response{LastModified: modified1, PollInterval: time.Minute}
	if cond.LastModified == modified1 {
		res.NotModified = true
		return page{}, res, nil
	}
	return page1, res, nil
}

func equal(a, b page) bool {
	return a.Next == b.Next && slices.Equal(a.Items, b.Items)
}

func TestListMiss(t *testing.T) {
	var got github.Conditional
	api := &fakeAPI{list: func(_ core.NotificationFilter, _ string, cond github.Conditional) (page, github.Response, error) {
		got = cond
		return page1, github.Response{LastModified: modified1}, nil
	}}
	s := New(api)
	if _, ok := s.Cached(Query{}); ok {
		t.Fatal("Cached reported a hit before any List")
	}

	p, err := s.List(t.Context(), Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !equal(p, page1) || got != (github.Conditional{}) {
		t.Errorf("List = %+v with %+v, want page1 fetched unconditionally", p, got)
	}
	if c, ok := s.Cached(Query{}); !ok || !equal(c, page1) {
		t.Errorf("Cached = %+v, %v; want page1", c, ok)
	}
}

func TestListFreshHit(t *testing.T) {
	api := &fakeAPI{list: servePage1}
	s := New(api)
	for range 2 {
		if _, err := s.List(t.Context(), Query{}); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	if n := api.lists.Load(); n != 1 {
		t.Errorf("API called %d times, want 1", n)
	}
}

func TestListStaleRevalidates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{list: servePage1}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.List(t.Context(), Query{}); err != nil {
			t.Fatalf("List: %v", err)
		}

		var got github.Conditional
		api.list = func(_ core.NotificationFilter, _ string, cond github.Conditional) (page, github.Response, error) {
			got = cond
			return page2, github.Response{LastModified: modified2}, nil
		}
		time.Sleep(2 * time.Minute)
		if c, ok := s.Cached(Query{}); !ok || !equal(c, page1) {
			t.Errorf("Cached while stale = %+v, %v; want page1", c, ok)
		}
		p, err := s.List(t.Context(), Query{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if got.LastModified != modified1 {
			t.Errorf("revalidated with %+v, want Last-Modified %q", got, modified1)
		}
		if !equal(p, page2) {
			t.Errorf("List = %+v, want page2", p)
		}
	})
}

func TestListNotModifiedRenews(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{list: servePage1}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.List(t.Context(), Query{}); err != nil {
			t.Fatalf("List: %v", err)
		}
		time.Sleep(2 * time.Minute)

		p, err := s.List(t.Context(), Query{})
		if err != nil {
			t.Fatalf("List after 304: %v", err)
		}
		if !equal(p, page1) {
			t.Errorf("List after 304 = %+v, want the cached page1", p)
		}
		if _, err := s.List(t.Context(), Query{}); err != nil {
			t.Fatalf("List: %v", err)
		}
		if n := api.lists.Load(); n != 2 {
			t.Errorf("API called %d times, want 2: the 304 should renew the entry", n)
		}
	})
}

func TestListQueries(t *testing.T) {
	type call struct {
		filter core.NotificationFilter
		cursor string
	}
	var calls []call
	api := &fakeAPI{list: func(filter core.NotificationFilter, cursor string, _ github.Conditional) (page, github.Response, error) {
		calls = append(calls, call{filter, cursor})
		if cursor != "" {
			return page2, github.Response{}, nil
		}
		return page1, github.Response{}, nil
	}}
	s := New(api)

	first, err := s.List(t.Context(), Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	second, err := s.List(t.Context(), Query{Cursor: first.Next})
	if err != nil {
		t.Fatalf("List next: %v", err)
	}
	all := Query{Filter: core.NotificationFilter{All: true, Participating: true}}
	if _, err := s.List(t.Context(), all); err != nil {
		t.Fatalf("List all: %v", err)
	}

	want := []call{{}, {cursor: page1.Next}, {filter: all.Filter}}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
	if !equal(second, page2) || !second.Last() {
		t.Errorf("second page = %+v, want page2 as the last page", second)
	}
	if c, _ := s.Cached(Query{}); !equal(c, page1) {
		t.Errorf("first page cached as %+v, want page1", c)
	}
}

func TestListError(t *testing.T) {
	api := &fakeAPI{list: func(core.NotificationFilter, string, github.Conditional) (page, github.Response, error) {
		return page{}, github.Response{}, fmt.Errorf("GET notifications: %w", core.ErrUnauthorized)
	}}
	s := New(api)

	_, err := s.List(t.Context(), Query{})
	if !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("error = %v, want it to match ErrUnauthorized", err)
	}
	if _, ok := s.Cached(Query{}); ok {
		t.Error("a failed List was cached")
	}
}

func TestPoll(t *testing.T) {
	api := &fakeAPI{list: servePage1}
	s := New(api)

	res, err := s.Poll(t.Context())
	if err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	if !res.Changed || res.Interval != time.Minute {
		t.Errorf("first Poll = %+v, want changed with a 1m interval", res)
	}

	// The entry is fresh, but Poll still asks, and GitHub says 304.
	res, err = s.Poll(t.Context())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if res.Changed || res.Interval != time.Minute {
		t.Errorf("Poll after 304 = %+v, want unchanged with a 1m interval", res)
	}
	if n := api.lists.Load(); n != 2 {
		t.Errorf("API called %d times, want 2", n)
	}
	if c, _ := s.Cached(Query{}); !equal(c, page1) {
		t.Errorf("Cached after 304 = %+v, want page1", c)
	}
}

func TestPollChanged(t *testing.T) {
	var got github.Conditional
	api := &fakeAPI{list: func(_ core.NotificationFilter, _ string, cond github.Conditional) (page, github.Response, error) {
		got = cond
		return page2, github.Response{LastModified: modified2, PollInterval: 90 * time.Second}, nil
	}}
	s := New(api)
	s.cache.Set(Query{}.key(), entry(page1, modified1))

	res, err := s.Poll(t.Context())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if !res.Changed || res.Interval != 90*time.Second {
		t.Errorf("Poll = %+v, want changed with a 90s interval", res)
	}
	if got.LastModified != modified1 {
		t.Errorf("polled with %+v, want Last-Modified %q", got, modified1)
	}

	p, err := s.List(t.Context(), Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !equal(p, page2) || api.lists.Load() != 1 {
		t.Errorf("List after Poll = %+v after %d calls, want page2 from the cache", p, api.lists.Load())
	}
}

func TestPollError(t *testing.T) {
	api := &fakeAPI{list: func(core.NotificationFilter, string, github.Conditional) (page, github.Response, error) {
		return page{}, github.Response{}, core.ErrRateLimited
	}}
	s := New(api)
	s.cache.Set(Query{}.key(), entry(page1, modified1))

	if _, err := s.Poll(t.Context()); !errors.Is(err, core.ErrRateLimited) {
		t.Errorf("error = %v, want it to match ErrRateLimited", err)
	}
	if c, ok := s.Cached(Query{}); !ok || !equal(c, page1) {
		t.Errorf("Cached after a failed Poll = %+v, %v; want page1", c, ok)
	}
}

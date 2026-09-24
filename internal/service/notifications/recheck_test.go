package notifications

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

func TestParseKey(t *testing.T) {
	for _, q := range []ListQuery{
		{},
		{Filter: core.NotificationFilter{All: true, Participating: true}, PageSize: 50, Cursor: "https://api.github.com/notifications?all=true&page=2"},
	} {
		want := q.normalize()
		if got, ok := parseKey(q.key()); !ok || got != want {
			t.Errorf("parseKey(%q) = %+v, %v; want %+v", q.key(), got, ok, want)
		}
	}
	for _, key := range []string{"", "notifications?all=true", "notifications?all=x&participating=false&page_size=30&cursor=", "list:o/r"} {
		if _, ok := parseKey(key); ok {
			t.Errorf("parseKey(%q) succeeded", key)
		}
	}
}

func TestKeptInboxRecheck(t *testing.T) {
	store := keptInbox(t)
	modified := modified1
	api := &fakeAPI{list: func(_ core.NotificationFilter, _ int, _ string, cond github.Conditional) (page, github.Response, error) {
		res := github.Response{LastModified: modified}
		if cond.LastModified == modified {
			res.NotModified = true
			return page{}, res, nil
		}
		return page2, res, nil
	}}
	s := New(api, WithStore(store))
	check := func() revalidate.Result {
		t.Helper()
		entries := s.Kept()
		if len(entries) != 1 || entries[0].Repo != (core.RepoRef{}) {
			t.Fatalf("Kept = %+v, want the inbox, of no repository", entries)
		}
		return entries[0].Check(t.Context())
	}

	if r := check(); r.Status != revalidate.NotModified || r.Sync != "" {
		t.Errorf("check = %+v, want not modified, with nothing to show", r)
	}
	modified = modified2
	if r := check(); r.Status != revalidate.Changed || r.Sync != SyncKey {
		t.Errorf("check after a change = %+v, want changed, reporting %q", r, SyncKey)
	}
	// The next session shows the new page without a request.
	api.lists.Store(0)
	p, err := New(api, WithStore(store)).List(t.Context(), inbox)
	if err != nil || p.Stale || !equal(p, page2) || api.lists.Load() != 0 {
		t.Errorf("List in the next session = %+v, %v after %d requests; want the new page, fresh", p, err, api.lists.Load())
	}
	if e := s.Kept()[0]; time.Since(e.CheckedAt) > time.Minute {
		t.Errorf("checked at %v, want now", e.CheckedAt)
	}
}

package issues

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

func TestParseKeys(t *testing.T) {
	cursor := "https://api.github.com/repositories/1/issues?page=2&per_page=30"
	for _, q := range []ListQuery{
		ListQuery{Repo: repo}.normalize(),
		{Repo: repo, State: core.FilterClosed, PageSize: 50, Cursor: cursor},
	} {
		if got, ok := parseListKey(listKey(q)); !ok || got != q {
			t.Errorf("parseListKey(listKey(%+v)) = %+v, %v", q, got, ok)
		}
	}
	for _, q := range []CommentsQuery{
		CommentsQuery{Repo: repo, Number: 7}.normalize(),
		{Repo: repo, Number: 12, PageSize: 10, Cursor: cursor},
	} {
		if got, ok := parseCommentsKey(commentsKey(q)); !ok || got != q {
			t.Errorf("parseCommentsKey(commentsKey(%+v)) = %+v, %v", q, got, ok)
		}
	}
	if r, n, ok := parseIssueKey("octo-org/hello#7"); !ok || r != repo || n != 7 {
		t.Errorf("parseIssueKey = %v, %d, %v", r, n, ok)
	}
	for _, key := range []string{"", "list:x", "list:nope:open:30:", "list:o/r:open:x:", "comments:o/r:30:", "comments:o/r#x:30:", "o/r#0", "o/r"} {
		if _, ok := parseListKey(key); ok {
			t.Errorf("parseListKey(%q) succeeded", key)
		}
		if _, ok := parseCommentsKey(key); ok {
			t.Errorf("parseCommentsKey(%q) succeeded", key)
		}
		if _, _, ok := parseIssueKey(key); ok {
			t.Errorf("parseIssueKey(%q) succeeded", key)
		}
	}
}

// check runs the checks of every kept entry and returns their results by
// ID.
func check(t *testing.T, s *Service) map[string]revalidate.Result {
	t.Helper()
	out := make(map[string]revalidate.Result)
	for _, e := range s.Kept() {
		if e.Repo != repo {
			t.Errorf("entry %s of %v, want %v", e.ID, e.Repo, repo)
		}
		out[e.ID] = e.Check(t.Context())
	}
	return out
}

func statuses(results map[string]revalidate.Result) map[revalidate.Status]int {
	n := make(map[revalidate.Status]int)
	for _, r := range results {
		n[r.Status]++
	}
	return n
}

func TestKeptNotModified(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	api := srv.api(t)
	s := New(api, WithStore(store))
	results := check(t, s)
	if n := statuses(results); len(results) != 3 || n[revalidate.NotModified] != 3 {
		t.Fatalf("results = %+v, want the list, the issue and the comments not modified", results)
	}
	for id, r := range results {
		if r.Sync != "" {
			t.Errorf("%s reports %q, want no change to show", id, r.Sync)
		}
	}
	api.checkCalls(t, "ListIssues", "GetIssue", "ListIssueComments")
	// Nothing was read into memory, and the kept entries count as
	// fetched now.
	if _, ok := s.CachedList(openSeven); ok {
		t.Error("the list is in memory, want it left alone")
	}
	for _, e := range s.Kept() {
		if time.Since(e.CheckedAt) > time.Minute {
			t.Errorf("%s checked at %v, want now", e.ID, e.CheckedAt)
		}
	}
}

func TestKeptChanged(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	api := srv.api(t)
	s := New(api, WithStore(store))
	// The list is on screen, stale, when the issue changes.
	listIssues(t, s, openSeven)
	s.Invalidate(repo)
	later := epoch.Add(time.Hour)
	srv.set(later, thread(3))

	results := check(t, s)
	if n := statuses(results); n[revalidate.Changed] != 3 {
		t.Fatalf("results = %+v, want every entry changed", results)
	}
	for id, r := range results {
		if r.Sync != SyncKey(repo) {
			t.Errorf("%s reports %q, want %q", id, r.Sync, SyncKey(repo))
		}
	}
	if p, ok := s.CachedList(openSeven); !ok || !p.Items[0].UpdatedAt.Equal(later) {
		t.Errorf("CachedList = %+v, want the new page in memory", p)
	}
	api.called()

	// The next session reads the new entries without a request.
	next := New(api, WithStore(store))
	listIssues(t, next, openSeven)
	readIssue(t, next)
	api.checkCalls(t)
	if c, _ := next.CachedComments(sevenComments); len(c.Items) != 3 {
		t.Errorf("comments = %d, want the 3 kept by the check", len(c.Items))
	}
}

func TestKeptRefusedIsForgotten(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)
	srv.fail(core.ErrNotFound)

	s := New(srv.api(t), WithStore(store))
	if n := statuses(check(t, s)); n[revalidate.Gone] != 3 {
		t.Errorf("statuses = %v, want every entry gone", n)
	}
	if ids := s.Kept(); len(ids) != 0 {
		t.Errorf("Kept after refusals = %+v, want nothing", ids)
	}
}

func TestKeptOfflinePauses(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)
	srv.fail(errDial)

	s := New(srv.api(t), WithStore(store))
	results := check(t, s)
	if n := statuses(results); n[revalidate.Offline] != 3 {
		t.Errorf("results = %+v, want every entry offline", results)
	}
	if got := len(s.Kept()); got != 3 {
		t.Errorf("%d entries kept while offline, want all 3", got)
	}
}

func TestKeptSkipsFresh(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	api := srv.api(t)
	s := New(api, WithStore(store))
	listIssues(t, s, openSeven)
	api.called()
	results := check(t, s)
	if r := results[kindList+":"+listKey(openSeven.normalize())]; r.Status != revalidate.Skipped {
		t.Errorf("fresh list = %+v, want skipped", r)
	}
	if calls := api.called(); slices.Contains(calls, "ListIssues") {
		t.Errorf("calls = %q, want no request for the fresh list", calls)
	}
}

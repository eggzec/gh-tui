package pulls

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// restThread serves the comments of #1 as REST does: with an ETag that
// changes with them, and a 304 to a request that has the current one.
type restThread struct {
	mu      sync.Mutex
	version int
	err     error
	// conds holds the ETag each request asked with.
	conds []string
}

func (r *restThread) etag() string { return `W/"v` + strconv.Itoa(r.version) + `"` }

// change adds a comment.
func (r *restThread) change() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version++
}

func (r *restThread) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *restThread) serve(_ context.Context, _ core.RepoRef, _ int, cursor string, _ int, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conds = append(r.conds, cond.ETag)
	if r.err != nil {
		return core.Page[core.Comment]{}, github.Response{}, r.err
	}
	if cond.ETag == r.etag() {
		return core.Page[core.Comment]{}, github.Response{StatusCode: 304, NotModified: true, ETag: r.etag()}, nil
	}
	p := core.Page[core.Comment]{}
	for i := range r.version + 1 {
		p.Items = append(p.Items, core.Comment{ID: "c" + strconv.Itoa(i) + cursor})
	}
	return p, github.Response{StatusCode: 200, ETag: r.etag(), URL: "https://api.github.com/repos/eggzec/gh-tui/issues/1/comments"}, nil
}

func (r *restThread) asked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.conds...)
}

func (r *restThread) api(v *versioned) *fakeAPI {
	api := v.api()
	api.restComments = r.serve
	return api
}

func TestParseCommentsKey(t *testing.T) {
	cursor := "https://api.github.com/repositories/1/issues/7/comments?page=2&per_page=30"
	for _, q := range []CommentsQuery{
		{Repo: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, Number: 7, PageSize: 30},
		{Repo: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, Number: 12, PageSize: 10, Cursor: cursor},
	} {
		if got, ok := parseCommentsKey(q.key()); !ok || got != q {
			t.Errorf("parseCommentsKey(%q) = %+v, %v; want %+v", q.key(), got, ok, q)
		}
	}
	// Keys come back in lower case, as they are made.
	if got, ok := parseCommentsKey(CommentsQuery{Repo: core.RepoRef{Owner: "Eggzec", Name: "GH-TUI"}, Number: 1}.key()); !ok || got.Repo.String() != "eggzec/gh-tui" || got.PageSize != defaultPageSize {
		t.Errorf("parseCommentsKey of a mixed case repository = %+v, %v", got, ok)
	}
	for _, key := range []string{
		"", "pull:eggzec/gh-tui#1", "pull:eggzec/gh-tui#x/comments?cursor=&first=30",
		"pull:eggzec/gh-tui#1/comments?cursor=&first=x", "pull:eggzec/gh-tui#0/comments?cursor=&first=30",
		"pull:eggzec/gh-tui#1/reviews?cursor=&first=30", "pull:nope#1/comments?cursor=&first=30",
		"pulls:eggzec/gh-tui?cursor=&first=30&state=open",
	} {
		if q, ok := parseCommentsKey(key); ok {
			t.Errorf("parseCommentsKey(%q) = %+v, want no query", key, q)
		}
	}
}

func TestCommentsRevalidated(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: epoch, checks: core.ChecksSuccess}
		thread := &restThread{}
		api := thread.api(v)
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Comments(t.Context(), firstComments); err != nil {
			t.Fatal(err)
		}

		// A stale page asks with its ETag, and a 304 keeps it, fresh.
		time.Sleep(2 * time.Minute)
		p, err := s.Comments(t.Context(), firstComments)
		if err != nil || len(p.Items) != 1 {
			t.Fatalf("Comments = %+v, %v; want the cached page", p, err)
		}
		if got := thread.asked(); len(got) != 2 || got[1] != `W/"v0"` {
			t.Errorf("asked with %q, want no ETag and then the page's", got)
		}
		if _, st := s.comments.Get(firstComments.key()); st != cache.Fresh {
			t.Errorf("page after a 304 is %v, want fresh", st)
		}

		// A new comment comes back in full.
		thread.change()
		time.Sleep(2 * time.Minute)
		if p, err := s.Comments(t.Context(), firstComments); err != nil || len(p.Items) != 2 {
			t.Errorf("Comments after a change = %+v, %v; want both comments", p, err)
		}
	})
}

func TestKeptCommentsRevalidatedInNewSession(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	thread := &restThread{}
	store := openStore(t)
	first := New(thread.api(v), WithStore(store))
	readDetail(t, first)

	// Without a list that vouches for it, the page kept an hour ago is
	// revalidated, for free.
	before := time.Now()
	s := New(thread.api(v), WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.Comments(t.Context(), firstComments)
	if err != nil || len(p.Items) != 1 {
		t.Fatalf("Comments = %+v, %v; want the kept page", p, err)
	}
	if got := thread.asked(); len(got) != 2 || got[1] != `W/"v0"` {
		t.Errorf("asked with %q, want the kept page's ETag", got)
	}
	// The kept page is marked fetched now, so the next session doesn't
	// ask again.
	if e, ok := New(v.api(), WithStore(store)).keptComments.Load(firstComments.key()); !ok || e.FetchedAt.Before(before) {
		t.Errorf("kept page fetched at %v, %v; want at least %v", e.FetchedAt, ok, before)
	}

	// Offline, the kept page is served as it is, marked.
	thread.fail(errDial)
	other := New(thread.api(v), WithStore(cachetest.Aged(store, time.Hour)))
	if p, err := other.Comments(t.Context(), firstComments); err != nil || !p.Offline || len(p.Items) != 1 {
		t.Errorf("Comments offline = %+v, %v; want the kept page, offline", p, err)
	}
}

// checkKept runs the check of every kept entry and returns their results by
// ID.
func checkKept(t *testing.T, s *Service) map[string]revalidate.Result {
	t.Helper()
	out := make(map[string]revalidate.Result)
	for _, e := range s.Kept() {
		if e.Repo.String() != repoID(repo) {
			t.Errorf("entry %s of %v, want %v", e.ID, e.Repo, repo)
		}
		out[e.ID] = e.Check(t.Context())
	}
	return out
}

func TestKeptComments(t *testing.T) {
	id := kindComments + ":" + firstComments.key()
	tests := []struct {
		name   string
		change func(*restThread)
		want   revalidate.Status
		sync   string
		kept   int
	}{
		{"not modified", func(*restThread) {}, revalidate.NotModified, "", 1},
		{"changed", (*restThread).change, revalidate.Changed, SyncKey(repo), 2},
		{"gone", func(r *restThread) { r.fail(&github.Error{StatusCode: 404}) }, revalidate.Gone, "", 0},
		{"offline", func(r *restThread) { r.fail(errDial) }, revalidate.Offline, "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &versioned{updated: epoch, checks: core.ChecksSuccess}
			thread := &restThread{}
			store := openStore(t)
			readDetail(t, New(thread.api(v), WithStore(store)))

			tt.change(thread)
			// The revalidator lists what a Catalog keeps, which the disk
			// store is.
			s := New(thread.api(v), WithStore(store))
			results := checkKept(t, s)
			if r, ok := results[id]; len(results) != 1 || !ok || r.Status != tt.want || r.Sync != tt.sync {
				t.Fatalf("results = %+v, want %s of %s with sync %q", results, tt.want, id, tt.sync)
			}
			e, ok := s.keptComments.Load(firstComments.key())
			if n := len(e.Value.Value.Items); tt.kept == 0 && ok || tt.kept > 0 && n != tt.kept {
				t.Errorf("kept page = %+v, %v; want %d comments", e.Value, ok, tt.kept)
			}
		})
	}
}

func TestKeptListsOnlyValidatedComments(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	// The fake's comments have no validators, and the details and lists
	// are GraphQL reads.
	s := New(v.api(), WithStore(store))
	list(t, s, openList)
	readDetail(t, s)
	if got := s.Kept(); len(got) != 0 {
		t.Errorf("Kept = %+v, want nothing without validators", got)
	}
}

func TestKeptCommentsOfOlderSchemaIsMiss(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	key := firstComments.key()
	// Before, the pages were GraphQL's, with a cursor REST can't read.
	old := cache.NewShelf[stampedComments](store, kindComments, commentsSchema-1)
	stale := stampedComments{Value: core.Page[core.Comment]{Items: []core.Comment{{ID: "old"}}, Next: "Y3Vyc29yOnYyOpHOBb002"}}
	if err := old.Save(key, cache.Entry[stampedComments]{Value: stale}); err != nil {
		t.Fatal(err)
	}
	api := v.api()
	p, err := New(api, WithStore(store)).Comments(t.Context(), firstComments)
	if err != nil || len(p.Items) != 1 || p.Items[0].ID != "c" {
		t.Errorf("Comments = %+v, %v; want the page read again", p, err)
	}
	if n := api.count("comments"); n != 1 {
		t.Errorf("comments called %d times, want 1", n)
	}
}

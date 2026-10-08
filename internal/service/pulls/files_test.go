package pulls

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// restFiles serves the files of #1 as REST does: an ETag that changes
// with them, and a 304 to a request that has the current one.
type restFiles struct {
	mu      sync.Mutex
	version int
	err     error
	conds   []string
	cursors []string
}

func (r *restFiles) etag() string { return fmt.Sprintf(`W/"f%d"`, r.version) }

func (r *restFiles) change() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version++
}

func (r *restFiles) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *restFiles) serve(_ context.Context, _ core.RepoRef, _ int, cursor string, cond github.Conditional) (core.Page[core.CommitFile], github.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conds = append(r.conds, cond.ETag)
	r.cursors = append(r.cursors, cursor)
	if r.err != nil {
		return core.Page[core.CommitFile]{}, github.Response{}, r.err
	}
	if cond.ETag == r.etag() {
		return core.Page[core.CommitFile]{}, github.Response{NotModified: true}, nil
	}
	p := core.Page[core.CommitFile]{Items: []core.CommitFile{{Path: fmt.Sprintf("f%d.go", r.version), Patch: "@@ -1 +1 @@\n-a\n+b"}}}
	return p, github.Response{ETag: r.etag()}, nil
}

func (r *restFiles) api() *fakeAPI { return &fakeAPI{files: r.serve} }

func (r *restFiles) requests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.conds)
}

var filesQ = FilesQuery{Repo: repo, Number: 1, Head: "abc123"}

func readFiles(t *testing.T, s *Service, q FilesQuery) core.Page[core.CommitFile] {
	t.Helper()
	p, err := s.Files(t.Context(), q)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	return p
}

func TestFilesMissThenHit(t *testing.T) {
	r := &restFiles{}
	s := New(r.api())
	if _, ok := s.CachedFiles(filesQ); ok {
		t.Fatal("CachedFiles before any read = true, want false")
	}
	p := readFiles(t, s, filesQ)
	if len(p.Items) != 1 || p.Items[0].Patch == "" {
		t.Fatalf("page = %+v, want the file with its patch", p)
	}
	if again := readFiles(t, s, filesQ); again.Items[0] != p.Items[0] {
		t.Errorf("second read = %+v, want %+v", again, p)
	}
	if n := r.requests(); n != 1 {
		t.Errorf("requests = %d, want 1: a fresh page is a hit", n)
	}
	if _, ok := s.CachedFiles(filesQ); !ok {
		t.Error("CachedFiles after a read = false, want true")
	}
}

func TestFilesNewHeadIsNewKey(t *testing.T) {
	r := &restFiles{}
	s := New(r.api())
	readFiles(t, s, filesQ)
	moved := filesQ
	moved.Head = "def456"
	if _, ok := s.CachedFiles(moved); ok {
		t.Fatal("CachedFiles of a new head = true, want false")
	}
	readFiles(t, s, moved)
	if n := r.requests(); n != 2 {
		t.Errorf("requests = %d, want 2", n)
	}
	// Pages of one head are told apart by their cursor.
	next := filesQ
	next.Cursor = "https://api.github.com/x?page=2"
	if _, ok := s.CachedFiles(next); ok {
		t.Error("CachedFiles of another cursor = true, want false")
	}
}

func TestFilesStaleIsRevalidatedFree(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &restFiles{}
		s := New(r.api(), WithTTL(time.Minute))
		readFiles(t, s, filesQ)

		// The same head can have a new diff, when the base moves, so a
		// stale page is asked about, and confirmed with a 304.
		time.Sleep(2 * time.Minute)
		p := readFiles(t, s, filesQ)
		if p.Items[0].Path != "f0.go" {
			t.Errorf("page = %+v, want the cached one after a 304", p)
		}
		if r.requests() != 2 || r.conds[1] != `W/"f0"` {
			t.Errorf("conds = %q, want the second request conditional", r.conds)
		}

		r.change()
		time.Sleep(2 * time.Minute)
		if p := readFiles(t, s, filesQ); p.Items[0].Path != "f1.go" {
			t.Errorf("page = %+v, want the changed diff of the same head", p)
		}
	})
}

func TestFilesInvalidate(t *testing.T) {
	r := &restFiles{}
	s := New(r.api())
	readFiles(t, s, filesQ)
	s.Invalidate(repo)
	if _, ok := s.CachedFiles(filesQ); !ok {
		t.Error("CachedFiles after Invalidate = false, want the stale page")
	}
	readFiles(t, s, filesQ)
	if r.requests() != 2 || r.conds[1] != `W/"f0"` {
		t.Errorf("conds = %q, want a conditional request after Invalidate", r.conds)
	}
}

func TestFilesKeptIsStaleThenAgain(t *testing.T) {
	r := &restFiles{}
	store := openStore(t)
	readFiles(t, New(r.api(), WithStore(cachetest.Aged(store, time.Hour))), filesQ)

	s := New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
	p := readFiles(t, s, filesQ)
	if !p.Stale || len(p.Items) != 1 {
		t.Fatalf("page in a new session = %+v, want the kept page, stale", p)
	}
	if r.requests() != 1 {
		t.Errorf("requests = %d, want none for the kept page", r.requests())
	}
	if again := readFiles(t, s, filesQ); !again.Stale {
		t.Error("second read = not stale, want the kept page again")
	}
	q := filesQ
	q.Again = true
	p = readFiles(t, s, q)
	if p.Stale || len(p.Items) != 1 {
		t.Errorf("Again read = %+v, want the page confirmed", p)
	}
	if r.requests() != 2 || r.conds[1] != `W/"f0"` {
		t.Errorf("conds = %q, want the Again read conditional, with the kept ETag", r.conds)
	}
}

func TestFilesKeptServedWhenOfflineOrLimited(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		limited bool
	}{
		{"offline", errDial, false},
		{"rate limited", fmt.Errorf("rest: %w", &core.RateLimitError{Reset: epoch}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &restFiles{}
			store := openStore(t)
			readFiles(t, New(r.api(), WithStore(cachetest.Aged(store, time.Hour))), filesQ)

			r.fail(tt.err)
			s := New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
			q := filesQ
			q.Again = true
			p := readFiles(t, s, q)
			if len(p.Items) != 1 || p.Limited != tt.limited || p.Offline == tt.limited {
				t.Errorf("page = Offline %v, Limited %v, %d items; want the kept page, limited = %v", p.Offline, p.Limited, len(p.Items), tt.limited)
			}
		})
	}
}

func TestFilesNotFoundHasNoPage(t *testing.T) {
	r := &restFiles{}
	r.fail(fmt.Errorf("get: %w", core.ErrNotFound))
	s := New(r.api())
	if _, err := s.Files(t.Context(), filesQ); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Files = %v, want not found", err)
	}
	if _, ok := s.CachedFiles(filesQ); ok {
		t.Error("a failed read was cached")
	}
}

func TestFilesTruncatedPageIsKept(t *testing.T) {
	api := &fakeAPI{files: func(context.Context, core.RepoRef, int, string, github.Conditional) (core.Page[core.CommitFile], github.Response, error) {
		return core.Page[core.CommitFile]{Items: []core.CommitFile{{Path: "a"}}, Truncated: true}, github.Response{ETag: `W/"t"`}, nil
	}}
	store := openStore(t)
	readFiles(t, New(api, WithStore(cachetest.Aged(store, time.Hour))), filesQ)
	p := readFiles(t, New(api, WithStore(cachetest.Aged(store, time.Hour))), filesQ)
	if !p.Truncated || !p.Stale {
		t.Errorf("kept page = %+v, want Truncated kept", p)
	}
}

func TestFilesChangedPageRevalidatesItsSiblings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &restFiles{}
		s := New(r.api(), WithTTL(time.Minute))
		second := filesQ
		second.Cursor = "https://api.github.com/x?page=2"
		readFiles(t, s, filesQ)
		time.Sleep(40 * time.Second)
		readFiles(t, s, second)

		// The first page is stale and the second fresh when the base moves.
		time.Sleep(30 * time.Second)
		r.change()
		if p := readFiles(t, s, filesQ); p.Items[0].Path != "f1.go" {
			t.Fatalf("first page = %+v, want the new diff", p)
		}
		before := r.requests()
		if p := readFiles(t, s, second); p.Items[0].Path != "f1.go" {
			t.Errorf("second page = %+v, want the new diff, not the old one beside the new first page", p)
		}
		if n := r.requests(); n != before+1 {
			t.Errorf("requests = %d, want the second page read again", n)
		}
	})
}

func TestFilesUnchangedPageLeavesSiblingsFresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &restFiles{}
		s := New(r.api(), WithTTL(time.Minute))
		second := filesQ
		second.Cursor = "https://api.github.com/x?page=2"
		readFiles(t, s, filesQ)
		time.Sleep(40 * time.Second)
		readFiles(t, s, second)
		time.Sleep(30 * time.Second)
		readFiles(t, s, filesQ) // a 304
		before := r.requests()
		readFiles(t, s, second)
		if n := r.requests(); n != before {
			t.Errorf("requests = %d, want the fresh sibling served from cache", n-before)
		}
	})
}

func TestFilesNeedHead(t *testing.T) {
	r := &restFiles{}
	s := New(r.api())
	q := filesQ
	q.Head = ""
	if _, err := s.Files(t.Context(), q); err == nil {
		t.Error("Files without a head = nil error, want one")
	}
	if _, ok := s.CachedFiles(q); ok {
		t.Error("CachedFiles without a head = true, want false")
	}
	if r.requests() != 0 {
		t.Error("a read without a head sent a request")
	}
}

func TestFilesKeyRoundTrip(t *testing.T) {
	q := FilesQuery{Repo: repo, Number: 12, Head: "ABC", Cursor: "https://api.github.com/repositories/1/pulls/12/files?per_page=100&page=2"}
	got, ok := parseFilesKey(q.key())
	if !ok || got.Number != 12 || got.Cursor != q.Cursor || got.Head != "abc" {
		t.Errorf("parseFilesKey = %+v, %v", got, ok)
	}
	if _, ok := parseFilesKey("pull:x#1"); ok {
		t.Error("parseFilesKey accepted another key")
	}
}

func TestKeptFiles(t *testing.T) {
	id := kindFiles + ":" + filesQ.key()
	tests := []struct {
		name   string
		change func(*restFiles)
		want   revalidate.Status
	}{
		{"not modified", func(*restFiles) {}, revalidate.NotModified},
		{"changed", (*restFiles).change, revalidate.Changed},
		{"gone", func(r *restFiles) { r.fail(&github.Error{StatusCode: 404}) }, revalidate.Gone},
		{"offline", func(r *restFiles) { r.fail(errDial) }, revalidate.Offline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &restFiles{}
			store := openStore(t)
			readFiles(t, New(r.api(), WithStore(store)), filesQ)

			tt.change(r)
			s := New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
			got := checkKept(t, s)
			if res, ok := got[id]; !ok || res.Status != tt.want {
				t.Errorf("check of %s = %+v (kept %v), want %v", id, res, got, tt.want)
			}
		})
	}
}

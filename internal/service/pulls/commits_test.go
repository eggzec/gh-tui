package pulls

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// restCommits serves the commits of #1 as REST does: an ETag that changes
// with them, and a 304 to a request that has the current one.
type restCommits struct {
	mu      sync.Mutex
	version int
	err     error
	conds   []string
}

func (r *restCommits) etag() string { return fmt.Sprintf(`W/"c%d"`, r.version) }

func (r *restCommits) change() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version++
}

func (r *restCommits) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *restCommits) serve(_ context.Context, _ core.RepoRef, _ int, cond github.Conditional) (core.Page[core.Commit], github.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conds = append(r.conds, cond.ETag)
	if r.err != nil {
		return core.Page[core.Commit]{}, github.Response{}, r.err
	}
	if cond.ETag == r.etag() {
		return core.Page[core.Commit]{}, github.Response{NotModified: true}, nil
	}
	p := core.Page[core.Commit]{Items: []core.Commit{{SHA: fmt.Sprintf("%040d", r.version), Subject: fmt.Sprintf("commit %d", r.version)}}}
	return p, github.Response{ETag: r.etag()}, nil
}

func (r *restCommits) api() *fakeAPI { return &fakeAPI{commits: r.serve} }

func (r *restCommits) requests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.conds)
}

var commitsQ = CommitsQuery{Repo: repo, Number: 1, Head: "abc123"}

func readCommits(t *testing.T, s *Service, q CommitsQuery) core.Page[core.Commit] {
	t.Helper()
	p, err := s.Commits(t.Context(), q)
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	return p
}

func TestCommitsMissThenHit(t *testing.T) {
	r := &restCommits{}
	s := New(r.api())
	if _, ok := s.CachedCommits(commitsQ); ok {
		t.Fatal("CachedCommits before any read = true, want false")
	}
	p := readCommits(t, s, commitsQ)
	if len(p.Items) != 1 || p.Items[0].Subject != "commit 0" {
		t.Fatalf("page = %+v, want the commit", p)
	}
	if again := readCommits(t, s, commitsQ); again.Items[0].SHA != p.Items[0].SHA {
		t.Errorf("second read = %+v, want %+v", again, p)
	}
	if n := r.requests(); n != 1 {
		t.Errorf("requests = %d, want 1: a fresh list is a hit", n)
	}
	if _, ok := s.CachedCommits(commitsQ); !ok {
		t.Error("CachedCommits after a read = false, want true")
	}
}

func TestCommitsNewHeadIsNewKey(t *testing.T) {
	r := &restCommits{}
	s := New(r.api())
	readCommits(t, s, commitsQ)
	q := commitsQ
	q.Head = "def456"
	if _, ok := s.CachedCommits(q); ok {
		t.Error("CachedCommits of a new head = true, want false")
	}
	readCommits(t, s, q)
	if r.requests() != 2 {
		t.Errorf("requests = %d, want one for each head", r.requests())
	}
	// The case of a SHA doesn't tell commits apart.
	q.Head = "DEF456"
	readCommits(t, s, q)
	if r.requests() != 2 {
		t.Errorf("requests = %d, want the upper case head to hit", r.requests())
	}
}

func TestCommitsStaleIsRevalidatedFree(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &restCommits{}
		s := New(r.api(), WithTTL(time.Minute))
		readCommits(t, s, commitsQ)

		time.Sleep(2 * time.Minute)
		p := readCommits(t, s, commitsQ)
		if p.Items[0].Subject != "commit 0" {
			t.Errorf("page = %+v, want the cached one after a 304", p)
		}
		if r.requests() != 2 || r.conds[1] != `W/"c0"` {
			t.Errorf("conds = %q, want the second request conditional", r.conds)
		}

		r.change()
		time.Sleep(2 * time.Minute)
		if p := readCommits(t, s, commitsQ); p.Items[0].Subject != "commit 1" {
			t.Errorf("page = %+v, want the changed list", p)
		}
	})
}

func TestCommitsInvalidate(t *testing.T) {
	r := &restCommits{}
	s := New(r.api())
	readCommits(t, s, commitsQ)
	s.Invalidate(repo)
	if _, ok := s.CachedCommits(commitsQ); !ok {
		t.Error("CachedCommits after Invalidate = false, want the stale list")
	}
	readCommits(t, s, commitsQ)
	if r.requests() != 2 || r.conds[1] != `W/"c0"` {
		t.Errorf("conds = %q, want a conditional request after Invalidate", r.conds)
	}
}

func TestCommitsKeptIsStaleThenAgain(t *testing.T) {
	r := &restCommits{}
	store := openStore(t)
	readCommits(t, New(r.api(), WithStore(cachetest.Aged(store, time.Hour))), commitsQ)

	s := New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
	p := readCommits(t, s, commitsQ)
	if !p.Stale || len(p.Items) != 1 {
		t.Fatalf("list in a new session = %+v, want the kept list, stale", p)
	}
	if again := readCommits(t, s, commitsQ); !again.Stale {
		t.Error("second read = not stale, want the kept list again")
	}
	q := commitsQ
	q.Again = true
	p = readCommits(t, s, q)
	if p.Stale || len(p.Items) != 1 {
		t.Errorf("Again read = %+v, want the list confirmed", p)
	}
	if r.requests() != 2 || r.conds[1] != `W/"c0"` {
		t.Errorf("conds = %q, want the Again read conditional, with the kept ETag", r.conds)
	}
}

func TestCommitsServedWhenOfflineOrLimited(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		offline bool
	}{
		{"offline", errDial, true},
		{"limited", fmt.Errorf("rest: %w", &core.RateLimitError{Reset: epoch}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := &restCommits{}
				s := New(r.api(), WithTTL(time.Minute))
				readCommits(t, s, commitsQ)
				time.Sleep(2 * time.Minute)
				r.fail(tt.err)
				p := readCommits(t, s, commitsQ)
				if len(p.Items) != 1 || p.Offline != tt.offline || p.Limited == tt.offline {
					t.Errorf("page = %+v, want the cached list, offline %v", p, tt.offline)
				}
			})
		})
	}
}

func TestCommitsNotFoundHasNoList(t *testing.T) {
	r := &restCommits{}
	r.fail(&github.Error{StatusCode: 404})
	s := New(r.api())
	if _, err := s.Commits(t.Context(), commitsQ); err == nil {
		t.Fatal("Commits of a missing pull request = nil error")
	}
	if _, ok := s.CachedCommits(commitsQ); ok {
		t.Error("CachedCommits after a failed read = true, want false")
	}
}

func TestCommitsNeedHead(t *testing.T) {
	r := &restCommits{}
	s := New(r.api())
	q := commitsQ
	q.Head = ""
	if _, err := s.Commits(t.Context(), q); !errors.Is(err, errNoHead) {
		t.Errorf("Commits without a head = %v, want errNoHead", err)
	}
	if _, ok := s.CachedCommits(q); ok {
		t.Error("CachedCommits without a head = true, want false")
	}
	if r.requests() != 0 {
		t.Errorf("requests = %d, want none", r.requests())
	}
}

func TestCommitsTruncatedIsKept(t *testing.T) {
	api := &fakeAPI{commits: func(context.Context, core.RepoRef, int, github.Conditional) (core.Page[core.Commit], github.Response, error) {
		return core.Page[core.Commit]{Items: []core.Commit{{SHA: "a"}}, Truncated: true}, github.Response{ETag: `W/"t"`}, nil
	}}
	s := New(api)
	if p := readCommits(t, s, commitsQ); !p.Truncated {
		t.Error("first read not Truncated")
	}
	if p := readCommits(t, s, commitsQ); !p.Truncated {
		t.Error("cached read not Truncated")
	}
}

func TestCommitsKeyRoundTrip(t *testing.T) {
	q := CommitsQuery{Repo: core.RepoRef{Owner: "Eggzec", Name: "gh-tui"}, Number: 12, Head: "ABC"}
	got, ok := parseCommitsKey(q.key())
	if !ok || got.Number != 12 || got.Head != "abc" || !strings.EqualFold(got.Repo.String(), q.Repo.String()) {
		t.Errorf("parseCommitsKey(%q) = %+v, %v", q.key(), got, ok)
	}
	for _, key := range []string{"", "pullfiles:eggzec/gh-tui#1@abc?cursor=", "pullcommits:eggzec/gh-tui#1@", "pullcommits:eggzec/gh-tui#x@abc", "pullcommits:nope#1@abc"} {
		if _, ok := parseCommitsKey(key); ok {
			t.Errorf("parseCommitsKey(%q) ok, want refused", key)
		}
	}
}

func TestKeptCommits(t *testing.T) {
	id := kindCommits + ":" + commitsQ.key()
	tests := []struct {
		name   string
		change func(*restCommits)
		want   revalidate.Status
	}{
		{"not modified", func(*restCommits) {}, revalidate.NotModified},
		{"changed", (*restCommits).change, revalidate.Changed},
		{"gone", func(r *restCommits) { r.fail(&github.Error{StatusCode: 404}) }, revalidate.Gone},
		{"offline", func(r *restCommits) { r.fail(errDial) }, revalidate.Offline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &restCommits{}
			store := openStore(t)
			readCommits(t, New(r.api(), WithStore(store)), commitsQ)

			tt.change(r)
			s := New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
			got := checkKept(t, s)
			if res, ok := got[id]; !ok || res.Status != tt.want {
				t.Errorf("check of %s = %+v (kept %v), want %v", id, res, got, tt.want)
			}
		})
	}
}

// commitFiles serves the files of one commit in pages of two, as GetCommit
// and ListCommitFiles do, and counts the requests.
type commitFiles struct {
	mu       sync.Mutex
	total    int
	truncate bool
	err      error
	reads    []string
}

const commitSHA = "0123456789abcdef0123456789abcdef01234567"

func (c *commitFiles) file(i int) core.CommitFile {
	return core.CommitFile{Path: fmt.Sprintf("f%d.go", i), SHA: fmt.Sprintf("blob%d", i), Patch: "@@ -1 +1 @@\n-a\n+b"}
}

func (c *commitFiles) page(from int) core.Page[core.CommitFile] {
	p := core.Page[core.CommitFile]{}
	for i := from; i < min(from+2, c.total); i++ {
		p.Items = append(p.Items, c.file(i))
	}
	if from+2 < c.total {
		p.Next = fmt.Sprintf("cursor%d", from+2)
	}
	return p
}

func (c *commitFiles) get(_ context.Context, _ core.RepoRef, sha string) (core.CommitDetail, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads = append(c.reads, "get "+sha)
	if c.err != nil {
		return core.CommitDetail{}, c.err
	}
	p := c.page(0)
	return core.CommitDetail{Files: p.Items, FilesNext: p.Next, FilesTruncated: c.truncate}, nil
}

func (c *commitFiles) list(_ context.Context, _ core.RepoRef, _, cursor string) (core.Page[core.CommitFile], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads = append(c.reads, "list "+cursor)
	if c.err != nil {
		return core.Page[core.CommitFile]{}, c.err
	}
	var from int
	_, _ = fmt.Sscanf(cursor, "cursor%d", &from)
	return c.page(from), nil
}

func (c *commitFiles) api() *fakeAPI { return &fakeAPI{commit: c.get, commitFiles: c.list} }

func (c *commitFiles) requests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reads)
}

var commitFilesQ = CommitFilesQuery{Repo: repo, SHA: commitSHA}

func TestCommitFilesReadsEveryPage(t *testing.T) {
	c := &commitFiles{total: 5}
	s := New(c.api())
	if _, ok := s.CachedCommitFiles(commitFilesQ); ok {
		t.Fatal("CachedCommitFiles before any read = true, want false")
	}
	p, err := s.CommitFiles(t.Context(), commitFilesQ)
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	if len(p.Items) != 5 || p.Next != "" || p.Truncated {
		t.Fatalf("page = %d files, Next %q, Truncated %v; want all 5 in one page", len(p.Items), p.Next, p.Truncated)
	}
	for i, f := range p.Items {
		if f != c.file(i) {
			t.Errorf("file %d = %+v, want %+v, with its blob SHA and patch", i, f, c.file(i))
		}
	}
	want := "get " + commitSHA + ",list cursor2,list cursor4"
	if got := strings.Join(c.reads, ","); got != want {
		t.Errorf("reads = %s, want %s", got, want)
	}
	if _, ok := s.CachedCommitFiles(commitFilesQ); !ok {
		t.Error("CachedCommitFiles after a read = false, want true")
	}
}

func TestCommitFilesNeverGoStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &commitFiles{total: 1}
		s := New(c.api(), WithTTL(time.Minute))
		if _, err := s.CommitFiles(t.Context(), commitFilesQ); err != nil {
			t.Fatalf("CommitFiles: %v", err)
		}
		time.Sleep(48 * time.Hour)
		s.Invalidate(repo)
		p, err := s.CommitFiles(t.Context(), commitFilesQ)
		if err != nil || len(p.Items) != 1 || p.Stale {
			t.Errorf("read after a long time = %+v, %v; want the same files, not stale", p, err)
		}
		if c.requests() != 1 {
			t.Errorf("requests = %d, want 1: what a SHA names is a hit for good", c.requests())
		}
	})
}

func TestCommitFilesShelfHit(t *testing.T) {
	c := &commitFiles{total: 3}
	store := openStore(t)
	if _, err := New(c.api(), WithStore(store)).CommitFiles(t.Context(), commitFilesQ); err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	before := c.requests()

	// A new session reads what the shelf kept, however old, without GitHub.
	s := New(c.api(), WithStore(cachetest.Aged(store, 30*24*time.Hour)))
	p, err := s.CommitFiles(t.Context(), commitFilesQ)
	if err != nil || len(p.Items) != 3 || p.Stale {
		t.Fatalf("read in a new session = %+v, %v; want the kept files", p, err)
	}
	if c.requests() != before {
		t.Errorf("requests = %d, want %d: the shelf answers", c.requests(), before)
	}
	if _, ok := s.CachedCommitFiles(commitFilesQ); !ok {
		t.Error("CachedCommitFiles after the shelf's hit = false, want true")
	}
}

func TestCommitFilesCaseOfSHA(t *testing.T) {
	c := &commitFiles{total: 1}
	s := New(c.api())
	if _, err := s.CommitFiles(t.Context(), commitFilesQ); err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	q := commitFilesQ
	q.SHA = strings.ToUpper(q.SHA)
	if _, err := s.CommitFiles(t.Context(), q); err != nil || c.requests() != 1 {
		t.Errorf("upper case SHA: %v, requests %d; want a hit", err, c.requests())
	}
}

func TestCommitFilesTruncated(t *testing.T) {
	c := &commitFiles{total: 3, truncate: true}
	p, err := New(c.api()).CommitFiles(t.Context(), commitFilesQ)
	if err != nil || !p.Truncated {
		t.Errorf("page = %+v, %v; want Truncated", p, err)
	}
}

func TestCommitFilesErrorsAreNotKept(t *testing.T) {
	tests := []struct {
		name string
		fail func(c *commitFiles)
	}{
		{"first page", func(c *commitFiles) { c.err = &github.Error{StatusCode: 404} }},
		{"offline", func(c *commitFiles) { c.err = errDial }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &commitFiles{total: 3}
			s := New(c.api(), WithStore(openStore(t)))
			tt.fail(c)
			if _, err := s.CommitFiles(t.Context(), commitFilesQ); err == nil {
				t.Fatal("CommitFiles = nil error, want the failure")
			}
			if _, ok := s.CachedCommitFiles(commitFilesQ); ok {
				t.Error("CachedCommitFiles after a failure = true, want false")
			}
			c.err = nil
			if p, err := s.CommitFiles(t.Context(), commitFilesQ); err != nil || len(p.Items) != 3 {
				t.Errorf("read after recovery = %+v, %v; want the files", p, err)
			}
		})
	}
}

func TestCommitFilesLaterPageFails(t *testing.T) {
	c := &commitFiles{total: 5}
	api := c.api()
	api.commitFiles = func(context.Context, core.RepoRef, string, string) (core.Page[core.CommitFile], error) {
		return core.Page[core.CommitFile]{}, errDial
	}
	s := New(api)
	if _, err := s.CommitFiles(t.Context(), commitFilesQ); !errors.Is(err, errDial) {
		t.Errorf("CommitFiles = %v, want the failure of the later page", err)
	}
	if _, ok := s.CachedCommitFiles(commitFilesQ); ok {
		t.Error("a partial list was cached")
	}
}

func TestCommitFilesNeedFullSHA(t *testing.T) {
	c := &commitFiles{total: 1}
	s := New(c.api())
	for _, sha := range []string{"", "main", "abc123", strings.Repeat("g", 40)} {
		q := commitFilesQ
		q.SHA = sha
		if _, err := s.CommitFiles(t.Context(), q); !errors.Is(err, errNotSHA) {
			t.Errorf("CommitFiles(%q) = %v, want errNotSHA", sha, err)
		}
	}
	if c.requests() != 0 {
		t.Errorf("requests = %d, want none", c.requests())
	}
}

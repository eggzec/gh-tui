package history

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

var repo = core.RepoRef{Owner: "octo-org", Name: "hello"}

// errDial is how a request fails while GitHub can't be reached.
var errDial = &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("connection refused")}

// errNotFound is how GitHub refuses what the account can't see.
var errNotFound = fmt.Errorf("github: 404 Not Found: %w", core.ErrNotFound)

// fakeGitHub is a repository with one linear history, of which the branch
// main points at the newest commit. It answers conditional requests with a
// 304 while nothing changed, pins the cursors of first pages as GitHub
// does, and fails every request with err while it is set. It records the
// calls.
type fakeGitHub struct {
	t testing.TB

	mu sync.Mutex
	// history is newest first.
	history []string
	err     error
	calls   []string
}

func newFake(tb testing.TB, commits int) *fakeGitHub {
	tb.Helper()
	f := &fakeGitHub{t: tb}
	for i := range commits {
		f.history = append([]string{sha(i)}, f.history...)
	}
	return f
}

// sha is the SHA of the i-th commit.
func sha(i int) string {
	return fmt.Sprintf("%040x", i+1)
}

// push adds a commit to main.
func (f *fakeGitHub) push() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history = append([]string{sha(len(f.history))}, f.history...)
}

func (f *fakeGitHub) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeGitHub) record(format string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	return f.err
}

// take returns the calls since the last take.
func (f *fakeGitHub) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	return calls
}

func (f *fakeGitHub) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	if got := f.take(); !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func (f *fakeGitHub) head() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.history[0]
}

// etag is the ETag of what main points at.
func (f *fakeGitHub) etag() string {
	return `"` + f.head()[36:] + `"`
}

func (f *fakeGitHub) ListBranches(_ context.Context, r core.RepoRef, cursor string, perPage int, cond github.Conditional) (core.Page[core.Branch], github.Response, error) {
	if err := f.record("branches %s %d %q %s", r, perPage, cursor, cond.ETag); err != nil {
		return core.Page[core.Branch]{}, github.Response{}, err
	}
	etag := f.etag()
	if cond.ETag == etag {
		return core.Page[core.Branch]{}, github.Response{StatusCode: 304, NotModified: true}, nil
	}
	p := core.Page[core.Branch]{Items: []core.Branch{{Name: "main", SHA: f.head(), Protected: true}}}
	return p, github.Response{StatusCode: 200, ETag: etag, URL: "https://api.github.com/repos/octo-org/hello/branches"}, nil
}

// ListCommits lists the history of main or of a SHA. A cursor is
// "sha=<head>&page=<n>&per_page=<size>".
func (f *fakeGitHub) ListCommits(_ context.Context, r core.RepoRef, ref, cursor string, perPage int, cond github.Conditional) (core.Page[core.Commit], github.Response, error) {
	if err := f.record("commits %s %s %d %q %s", r, ref, perPage, cursor, cond.ETag); err != nil {
		return core.Page[core.Commit]{}, github.Response{}, err
	}
	f.mu.Lock()
	history := slices.Clone(f.history)
	f.mu.Unlock()

	start, page := ref, 1
	if ref == "" || ref == "main" {
		start = history[0]
	}
	if cursor != "" {
		q, err := url.ParseQuery(cursor)
		if err != nil {
			f.t.Fatalf("cursor %q: %v", cursor, err)
		}
		start = q.Get("sha")
		page, _ = strconv.Atoi(q.Get("page"))
		perPage, _ = strconv.Atoi(q.Get("per_page"))
	}
	var res github.Response
	if cursor == "" && !isSHA(ref) {
		res.ETag = f.etag()
		if cond.ETag == res.ETag {
			return core.Page[core.Commit]{}, github.Response{StatusCode: 304, NotModified: true}, nil
		}
	}
	i := slices.Index(history, start)
	if i < 0 {
		return core.Page[core.Commit]{}, github.Response{}, errNotFound
	}
	walk := history[i:]
	lo, hi := min((page-1)*perPage, len(walk)), min(page*perPage, len(walk))
	var p core.Page[core.Commit]
	for j, s := range walk[lo:hi] {
		c := core.Commit{SHA: s, Subject: "commit " + s[36:]}
		if lo+j+1 < len(walk) {
			c.Parents = []string{walk[lo+j+1]}
		}
		p.Items = append(p.Items, c)
	}
	if hi < len(walk) {
		p.Next = "sha=" + walk[0] + "&page=" + strconv.Itoa(page+1) + "&per_page=" + strconv.Itoa(perPage)
	}
	res.StatusCode = 200
	return p, res, nil
}

func (f *fakeGitHub) GetCommit(_ context.Context, r core.RepoRef, s string) (core.CommitDetail, error) {
	if err := f.record("commit %s %s", r, s); err != nil {
		return core.CommitDetail{}, err
	}
	return core.CommitDetail{
		SHA: s, Subject: "commit " + s[36:],
		Stats:     core.CommitStats{Additions: 1, Total: 1},
		Files:     []core.CommitFile{{Path: "a.go", Status: core.FileModified, Additions: 1, Patch: "@@ -1 +1,2 @@\n a\n+b"}},
		FilesNext: "page=2",
	}, nil
}

func (f *fakeGitHub) ListCommitFiles(_ context.Context, r core.RepoRef, s, cursor string) (core.Page[core.CommitFile], error) {
	if err := f.record("files %s %s %s", r, s, cursor); err != nil {
		return core.Page[core.CommitFile]{}, err
	}
	return core.Page[core.CommitFile]{Items: []core.CommitFile{{Path: "b.go", Status: core.FileAdded, PatchTruncated: true}}}, nil
}

func (f *fakeGitHub) Compare(_ context.Context, r core.RepoRef, base, head string, cond github.Conditional) (core.Compare, github.Response, error) {
	if err := f.record("compare %s %s...%s %s", r, base, head, cond.ETag); err != nil {
		return core.Compare{}, github.Response{}, err
	}
	etag := f.etag()
	if cond.ETag == etag {
		return core.Compare{}, github.Response{StatusCode: 304, NotModified: true}, nil
	}
	if strings.Contains(base, "nope") {
		return core.Compare{}, github.Response{}, errNotFound
	}
	f.mu.Lock()
	ahead := slices.Index(f.history, base)
	f.mu.Unlock()
	return core.Compare{Status: core.CompareAhead, AheadBy: ahead}, github.Response{StatusCode: 200, ETag: etag}, nil
}

// stores returns a fresh store of the account's entries and one of
// objects, on disk.
func stores(tb testing.TB) (entries, objects *disk.Store) {
	tb.Helper()
	entries, err := disk.Open(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	objects, err = disk.Open(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	return entries, objects
}

// newService returns a service over f that keeps entries and objects in
// the stores.
func newService(f *fakeGitHub, entries, objects cache.Store) *Service {
	return New(f, WithStore(entries), WithObjects(objects))
}

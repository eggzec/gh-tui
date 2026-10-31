package autolinks

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

type fakeAPI struct {
	calls  atomic.Int32
	notMod bool
	err    error

	mu       sync.Mutex
	lastCond github.Conditional
}

func (f *fakeAPI) cond() github.Conditional {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastCond
}

func (f *fakeAPI) VerifyIssue(_ context.Context, repo core.RepoRef, n int, cond github.Conditional) (core.Verified, github.Response, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.lastCond = cond
	f.mu.Unlock()
	if f.err != nil {
		return core.Verified{}, github.Response{}, f.err
	}
	if f.notMod {
		return core.Verified{}, github.Response{NotModified: true}, nil
	}
	if n == 404 {
		return core.Verified{}, github.Response{}, nil
	}
	return core.Verified{Exists: true, URL: "https://github.com/" + repo.Owner + "/" + repo.Name + "/pull/1", Kind: core.VerifiedPull}, github.Response{ETag: `"e"`}, nil
}

func (f *fakeAPI) VerifyUser(context.Context, string, github.Conditional) (core.Verified, github.Response, error) {
	f.calls.Add(1)
	return core.Verified{Exists: true, URL: "https://github.com/octo", Kind: core.VerifiedUser}, github.Response{}, nil
}

func (f *fakeAPI) VerifyCommit(context.Context, core.RepoRef, string, github.Conditional) (core.Verified, github.Response, error) {
	f.calls.Add(1)
	return core.Verified{}, github.Response{}, nil
}

func TestVerify(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	var published []string
	s := New(api, WithPublish(func(k string) { published = append(published, k) }), WithHost("github.com"))
	cands := []Candidate{
		{Kind: core.VerifiedIssue, Ref: "1"},
		{Kind: core.VerifiedIssue, Ref: "1"},
		{Kind: core.VerifiedIssue, Ref: "404"},
		{Kind: core.VerifiedUser, Ref: "octo"},
		{Kind: core.VerifiedCommit, Ref: "abcdef1"},
		{Kind: core.VerifiedIssue, Ref: "2", Repo: "bad"},
	}
	if got := s.Cached("o/r", cands); got[0].Known {
		t.Fatal("Cached knew something before Verify")
	}
	if api.calls.Load() != 0 {
		t.Fatal("Cached did I/O")
	}
	res := s.Verify(context.Background(), "o/r", cands)
	if n := api.calls.Load(); n != 4 {
		t.Fatalf("calls = %d, want 4 (duplicates and bad repos skipped)", n)
	}
	if !res[0].Exists || res[0].Kind != core.VerifiedPull || !res[1].Known {
		t.Errorf("hit: %+v %+v", res[0], res[1])
	}
	if !res[2].Known || res[2].Exists || res[2].Err != nil {
		t.Errorf("miss should be known and not an error: %+v", res[2])
	}
	if res[5].Known {
		t.Errorf("candidate without a repository was verified: %+v", res[5])
	}
	if len(published) != 1 || published[0] != SyncKey("github.com") {
		t.Errorf("published = %v", published)
	}
	// Fresh answers, misses too, cost no request.
	s.Verify(context.Background(), "o/r", cands)
	if n := api.calls.Load(); n != 4 {
		t.Errorf("calls after a second Verify = %d, want 4", n)
	}
	if len(published) != 1 {
		t.Errorf("a batch that learned nothing published: %v", published)
	}
}

func TestVerifyRevalidates(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	var published int
	// A TTL this short leaves every answer stale at once.
	s := New(api, WithTTL(time.Nanosecond), WithPublish(func(string) { published++ }))
	cands := []Candidate{{Kind: core.VerifiedIssue, Ref: "1"}}
	first := s.Verify(context.Background(), "o/r", cands)
	if !first[0].Exists || published != 1 {
		t.Fatalf("first: %+v published %d", first[0], published)
	}
	api.notMod = true
	res := s.Verify(context.Background(), "o/r", cands)
	if got := api.cond().ETag; got != `"e"` {
		t.Errorf("revalidation sent ETag %q, want %q", got, `"e"`)
	}
	if !res[0].Known || !res[0].Exists || res[0].URL != first[0].URL || res[0].Err != nil {
		t.Errorf("a 304 changed the answer: %+v", res[0])
	}
	if published != 1 {
		t.Errorf("a 304 published: %d", published)
	}
}

func TestVerifyIgnoresBadIssueRef(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	s := New(api)
	res := s.Verify(context.Background(), "o/r", []Candidate{{Kind: core.VerifiedIssue, Ref: "abc"}})
	if api.calls.Load() != 0 || res[0].Known || res[0].Err != nil {
		t.Errorf("calls %d, result %+v", api.calls.Load(), res[0])
	}
}

func TestVerifyError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	s := New(&fakeAPI{err: boom})
	res := s.Verify(context.Background(), "o/r", []Candidate{{Kind: core.VerifiedIssue, Ref: "1"}})
	if res[0].Known || !errors.Is(res[0].Err, boom) {
		t.Errorf("result = %+v", res[0])
	}
}

package history

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

func shas(p core.Page[core.Commit]) []string {
	out := make([]string, len(p.Items))
	for i := range p.Items {
		out[i] = p.Items[i].SHA
	}
	return out
}

func TestBranches(t *testing.T) {
	f := newFake(t, 3)
	entries, objects := stores(t)
	s := newService(f, entries, objects)
	q := BranchesQuery{Repo: repo}

	if _, ok := s.CachedBranches(q); ok {
		t.Error("CachedBranches hit before a read")
	}
	p, err := s.Branches(t.Context(), q)
	if err != nil || len(p.Items) != 1 || p.Items[0].SHA != sha(2) || p.Stale {
		t.Fatalf("Branches = %+v, %v", p, err)
	}
	f.wantCalls(t, `branches octo-org/hello 100 "" `)
	if got, ok := s.CachedBranches(q); !ok || got.Items[0].Name != "main" {
		t.Errorf("CachedBranches = %+v, %v", got, ok)
	}
	if _, err := s.Branches(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	f.wantCalls(t)

	// A later session shows the kept page at once, then revalidates it.
	later := newService(f, cachetest.Aged(entries, time.Hour), objects)
	p, err = later.Branches(t.Context(), q)
	if err != nil || !p.Stale || p.Items[0].SHA != sha(2) {
		t.Fatalf("kept Branches = %+v, %v; want the kept page, stale", p, err)
	}
	f.wantCalls(t)
	if p, err = later.Branches(t.Context(), q.again()); err != nil || p.Stale {
		t.Fatalf("Branches = %+v, %v", p, err)
	}
	f.wantCalls(t, `branches octo-org/hello 100 "" "0003"`)
}

func TestCommitsPages(t *testing.T) {
	f := newFake(t, 5)
	entries, objects := stores(t)
	s := newService(f, entries, objects)
	q := CommitsQuery{Repo: repo, Ref: "main", PageSize: 2}

	first, err := s.Commits(t.Context(), q)
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if want := []string{sha(4), sha(3)}; !slices.Equal(shas(first), want) || first.Next == "" {
		t.Fatalf("first page = %v next %q, want %v and more", shas(first), first.Next, want)
	}
	f.wantCalls(t, `commits octo-org/hello main 2 "" `)

	// A push between two pages doesn't shift the second: it continues
	// the history the first showed.
	f.push()
	second, err := s.Commits(t.Context(), CommitsQuery{Repo: repo, Ref: "main", Cursor: first.Next, PageSize: 2})
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if want := []string{sha(2), sha(1)}; !slices.Equal(shas(second), want) {
		t.Errorf("second page = %v, want %v", shas(second), want)
	}
	f.wantCalls(t, fmt.Sprintf("commits octo-org/hello main 2 %q ", first.Next))

	// A page that a SHA names is cached for good, also across sessions,
	// without a request.
	q2 := CommitsQuery{Repo: repo, Cursor: first.Next, PageSize: 2}
	if _, ok := s.CachedCommits(q2); !ok {
		t.Error("CachedCommits missed the second page")
	}
	later := newService(f, cachetest.Aged(entries, time.Hour), objects)
	if got, err := later.Commits(t.Context(), q2); err != nil || !slices.Equal(shas(got), shas(second)) {
		t.Errorf("kept second page = %v, %v", shas(got), err)
	}
	f.wantCalls(t)
}

func TestCommitsRefPageRevalidates(t *testing.T) {
	f := newFake(t, 3)
	entries, objects := stores(t)
	q := CommitsQuery{Repo: repo, Ref: "main"}
	if _, err := newService(f, entries, objects).Commits(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	f.take()

	// A new session shows the kept first page, stale, and revalidates it
	// with a free 304.
	s := newService(f, cachetest.Aged(entries, time.Hour), objects)
	p, err := s.Commits(t.Context(), q)
	if err != nil || !p.Stale || p.Items[0].SHA != sha(2) {
		t.Fatalf("kept page = %+v, %v", p, err)
	}
	f.wantCalls(t)
	if p, err = s.Commits(t.Context(), q.again()); err != nil || p.Stale {
		t.Fatalf("Commits = %+v, %v", p, err)
	}
	f.wantCalls(t, `commits octo-org/hello main 50 "" "0003"`)

	// Once the ref moves, the first page starts at the new head.
	f.push()
	s.Invalidate(repo)
	if p, err = s.Commits(t.Context(), q); err != nil || p.Items[0].SHA != sha(3) {
		t.Fatalf("moved page = %v, %v; want %s first", shas(p), err, sha(3))
	}
	f.wantCalls(t, `commits octo-org/hello main 50 "" "0003"`)
}

func TestCommitsBySHA(t *testing.T) {
	f := newFake(t, 3)
	s := New(f)
	q := CommitsQuery{Repo: repo, Ref: sha(1)}
	for range 2 {
		p, err := s.Commits(t.Context(), q)
		if err != nil || !slices.Equal(shas(p), []string{sha(1), sha(0)}) {
			t.Fatalf("Commits = %v, %v", shas(p), err)
		}
	}
	// A SHA's history never changes, so there is one request, never
	// conditional, and it isn't revalidated even once invalidated.
	s.Invalidate(repo)
	if _, err := s.Commits(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	f.wantCalls(t, fmt.Sprintf(`commits octo-org/hello %s 50 "" `, sha(1)))
}

func TestCommitsOffline(t *testing.T) {
	f := newFake(t, 2)
	s := New(f)
	q := CommitsQuery{Repo: repo}
	if _, err := s.Commits(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	s.Invalidate(repo)
	f.fail(errDial)
	p, err := s.Commits(t.Context(), q)
	if err != nil || !p.Offline || p.Items[0].SHA != sha(1) {
		t.Errorf("offline Commits = %+v, %v; want the last page, offline", p, err)
	}

	f.fail(&core.RateLimitError{Reset: time.Now().Add(time.Hour)})
	p, err = s.Commits(t.Context(), q)
	if err != nil || p.Offline || !p.Limited || p.Items[0].SHA != sha(1) {
		t.Errorf("rate-limited Commits = %+v, %v; want the last page, limited", p, err)
	}

	// Once GitHub answers, with a 304, the page is served unmarked.
	f.fail(nil)
	f.take()
	p, err = s.Commits(t.Context(), q)
	if err != nil || p.Offline || p.Limited || p.Items[0].SHA != sha(1) {
		t.Errorf("Commits after the outage = %+v, %v; want the last page, unmarked", p, err)
	}
	if calls := f.take(); len(calls) != 1 {
		t.Errorf("calls after the outage = %q, want one 304", calls)
	}
}

func TestCommitsRefusedDropsKept(t *testing.T) {
	f := newFake(t, 2)
	entries, objects := stores(t)
	q := CommitsQuery{Repo: repo, Ref: "main"}
	if _, err := newService(f, entries, objects).Commits(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	f.fail(errNotFound)
	s := newService(f, cachetest.Aged(entries, time.Hour), objects)
	// The kept page shows at once; the revalidation that GitHub refuses
	// drops it.
	if _, err := s.Commits(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commits(t.Context(), q.again()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("err = %v, want core.ErrNotFound", err)
	}
	f.fail(nil)
	f.take()
	p, err := newService(f, entries, objects).Commits(t.Context(), q)
	if err != nil || p.Stale {
		t.Errorf("Commits = %+v, %v; want a fresh read", p, err)
	}
	f.wantCalls(t, `commits octo-org/hello main 50 "" `)
}

func TestCommit(t *testing.T) {
	f := newFake(t, 2)
	entries, objects := stores(t)
	s := newService(f, entries, objects)

	d, err := s.Commit(t.Context(), repo, sha(1))
	if err != nil || d.SHA != sha(1) || len(d.Files) != 1 || d.FilesNext == "" {
		t.Fatalf("Commit = %+v, %v", d, err)
	}
	if _, err := s.Commit(t.Context(), repo, sha(1)); err != nil {
		t.Fatal(err)
	}
	f.wantCalls(t, "commit octo-org/hello "+sha(1))
	if got, ok := s.CachedCommit(repo, sha(1)); !ok || got.SHA != sha(1) {
		t.Errorf("CachedCommit = %+v, %v", got, ok)
	}

	// A later session reads it from the object store.
	later := newService(f, entries, objects)
	if got, err := later.Commit(t.Context(), repo, sha(1)); err != nil || got.Files[0].Patch != d.Files[0].Patch || got.Stats != d.Stats {
		t.Errorf("kept Commit = %+v, %v", got, err)
	}
	f.wantCalls(t)

	if _, err := s.Commit(t.Context(), repo, "main"); !errors.Is(err, errNotSHA) {
		t.Errorf("Commit by a branch = %v, want errNotSHA", err)
	}
	f.wantCalls(t)
}

func TestCommitFiles(t *testing.T) {
	f := newFake(t, 1)
	entries, objects := stores(t)
	s := newService(f, entries, objects)

	first, err := s.CommitFiles(t.Context(), CommitFilesQuery{Repo: repo, SHA: sha(0)})
	if err != nil || len(first.Items) != 1 || first.Items[0].Path != "a.go" || first.Next != "page=2" {
		t.Fatalf("first files = %+v, %v; want the detail's", first, err)
	}
	q := CommitFilesQuery{Repo: repo, SHA: sha(0), Cursor: first.Next}
	for range 2 {
		p, err := s.CommitFiles(t.Context(), q)
		if err != nil || p.Items[0].Path != "b.go" || !p.Items[0].PatchTruncated {
			t.Fatalf("second files = %+v, %v", p, err)
		}
	}
	f.wantCalls(t, "commit octo-org/hello "+sha(0), "files octo-org/hello "+sha(0)+" page=2")
	if p, ok := s.CachedCommitFiles(q); !ok || p.Items[0].Path != "b.go" {
		t.Errorf("CachedCommitFiles = %+v, %v", p, ok)
	}
	if p, ok := s.CachedCommitFiles(CommitFilesQuery{Repo: repo, SHA: sha(0)}); !ok || p.Items[0].Path != "a.go" {
		t.Errorf("CachedCommitFiles of the first page = %+v, %v", p, ok)
	}
	if _, err := newService(f, entries, objects).CommitFiles(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	f.wantCalls(t)
}

func TestCompare(t *testing.T) {
	f := newFake(t, 4)
	s := New(f)

	c, err := s.Compare(t.Context(), repo, sha(1), "main")
	if want := (core.Compare{Status: core.CompareAhead, AheadBy: 2}); err != nil || c != want {
		t.Fatalf("Compare = %+v, %v; want %+v", c, err, want)
	}
	if got, ok := s.CachedCompare(repo, sha(1), "main"); !ok || got != c {
		t.Errorf("CachedCompare = %+v, %v", got, ok)
	}
	s.Invalidate(repo)
	if got, err := s.Compare(t.Context(), repo, sha(1), "main"); err != nil || got != c {
		t.Errorf("revalidated Compare = %+v, %v", got, err)
	}
	f.wantCalls(t, "compare octo-org/hello "+sha(1)+"...main ", "compare octo-org/hello "+sha(1)+`...main "0004"`)

	if _, err := s.Compare(t.Context(), repo, "nope", "main"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want core.ErrNotFound", err)
	}
}

func TestKept(t *testing.T) {
	f := newFake(t, 3)
	entries, objects := stores(t)
	s := newService(f, entries, objects)
	ctx := t.Context()
	if _, err := s.Branches(ctx, BranchesQuery{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	first, err := s.Commits(ctx, CommitsQuery{Repo: repo, Ref: "main", PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Neither of these is listed: a SHA names them.
	if _, err := s.Commits(ctx, CommitsQuery{Repo: repo, Cursor: first.Next, PageSize: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit(ctx, repo, sha(0)); err != nil {
		t.Fatal(err)
	}
	f.take()

	check := func() map[string]revalidate.Result {
		t.Helper()
		out := make(map[string]revalidate.Result)
		for _, e := range newService(f, entries, objects).Kept() {
			if e.Repo != repo {
				t.Errorf("entry %s of %v, want %v", e.ID, e.Repo, repo)
			}
			out[e.ID] = e.Check(ctx)
		}
		return out
	}
	ids := []string{"branchlist:branches:octo-org/hello:100:", "commitlist:commits:octo-org/hello:2:main"}

	got := check()
	for _, id := range ids {
		if r, ok := got[id]; !ok || r.Status != revalidate.NotModified {
			t.Errorf("%s = %+v, %v; want not modified", id, r, ok)
		}
	}
	if len(got) != len(ids) {
		t.Errorf("kept %d entries, want %d: %v", len(got), len(ids), got)
	}

	f.push()
	for id, r := range check() {
		if r.Status != revalidate.Changed || r.Sync != SyncKey(repo) {
			t.Errorf("%s after a push = %+v, want changed with %q", id, r, SyncKey(repo))
		}
	}
	// The changed page is kept, so a later session starts from it.
	p, err := newService(f, cachetest.Aged(entries, time.Hour), objects).Commits(ctx, CommitsQuery{Repo: repo, Ref: "main", PageSize: 2})
	if err != nil || p.Items[0].SHA != sha(3) {
		t.Errorf("kept page after a recheck = %v, %v; want %s first", shas(p), err, sha(3))
	}
}

func TestParseKeys(t *testing.T) {
	for _, q := range []BranchesQuery{
		BranchesQuery{Repo: repo}.normalize(),
		{Repo: repo, PageSize: 30, Cursor: "https://api.github.com/repositories/1/branches?page=2"},
	} {
		if got, ok := parseBranchesKey(branchesKey(q)); !ok || got != q {
			t.Errorf("parseBranchesKey(branchesKey(%+v)) = %+v, %v", q, got, ok)
		}
	}
	for _, q := range []CommitsQuery{
		CommitsQuery{Repo: repo}.normalize(),
		{Repo: repo, Ref: "feat/x", PageSize: 20},
	} {
		if got, ok := parseRefPageKey(refPageKey(q)); !ok || got != q {
			t.Errorf("parseRefPageKey(refPageKey(%+v)) = %+v, %v", q, got, ok)
		}
	}
	for _, key := range []string{"", "branches:x", "branches:nope:1:", "branches:o/r:x:", "commits:o/r:0:main", "commits:o/r:50:" + sha(1)} {
		if _, ok := parseBranchesKey(key); ok {
			t.Errorf("parseBranchesKey(%q) succeeded", key)
		}
		if _, ok := parseRefPageKey(key); ok {
			t.Errorf("parseRefPageKey(%q) succeeded", key)
		}
	}
}

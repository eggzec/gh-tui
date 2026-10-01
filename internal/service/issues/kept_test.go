package issues

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// keptServer is a fake GitHub with issue 7 and its comments, which answers
// conditional requests with a 304 while nothing changed, and fails every
// request with err while it is set.
type keptServer struct {
	mu       sync.Mutex
	updated  time.Time
	comments []core.Comment
	err      error
}

func (k *keptServer) set(updated time.Time, comments []core.Comment) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.updated, k.comments = updated, comments
}

func (k *keptServer) fail(err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.err = err
}

func (k *keptServer) state() (core.Issue, []core.Comment, string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	it := issue(7)
	it.UpdatedAt = k.updated
	return it, k.comments, fmt.Sprintf(`"%d-%d"`, k.updated.Unix(), len(k.comments)), k.err
}

// api returns a fake that answers from k.
func (k *keptServer) api(t *testing.T) *fakeAPI {
	t.Helper()
	answer := func(cond github.Conditional) (github.Response, error) {
		_, _, etag, err := k.state()
		switch {
		case err != nil:
			return github.Response{}, err
		case cond.ETag == etag:
			return notModified, nil
		}
		res := ok(etag)
		res.URL = "https://api.github.com/repos/octo-org/hello/issues"
		return res, nil
	}
	return &fakeAPI{
		t: t,
		listIssues: func(_ core.StateFilter, _ string, _ int, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
			it, _, _, _ := k.state()
			res, err := answer(cond)
			return core.Page[core.Issue]{Items: []core.Issue{it}}, res, err
		},
		getIssue: func(_ int, cond github.Conditional) (core.Issue, github.Response, error) {
			it, _, _, _ := k.state()
			res, err := answer(cond)
			return it, res, err
		},
		listComments: func(_ int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
			_, all, _, _ := k.state()
			res, err := answer(cond)
			return commentPage(t, all, cursor, perPage), res, err
		},
	}
}

func openStore(t *testing.T) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

var openSeven = ListQuery{Repo: repo}

// firstSession reads the list, issue 7 and its comments from srv into
// store, the way a session that opened the issue does.
func firstSession(t *testing.T, srv *keptServer, store *disk.Store) {
	t.Helper()
	s := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
	listIssues(t, s, openSeven)
	readIssue(t, s)
}

func TestKeptListIsServedStaleThenRevalidated(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	api := srv.api(t)
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.List(t.Context(), openSeven)
	if err != nil || !p.Stale || p.Offline || !equalNumbers(numbers(p), 7) {
		t.Fatalf("List in a new session = %+v, %v; want the kept page, stale", p, err)
	}
	api.checkCalls(t)
	if got, ok := s.CachedList(openSeven); !ok || got.Stale || !equalNumbers(numbers(got), 7) {
		t.Errorf("CachedList = %+v, %v; want the kept page in memory", got, ok)
	}

	// Reading it again asks GitHub with the kept ETag, which is free.
	p, err = s.List(t.Context(), openSeven.again())
	if err != nil || p.Stale || !equalNumbers(numbers(p), 7) {
		t.Fatalf("second List = %+v, %v; want the page revalidated", p, err)
	}
	api.checkCalls(t, "ListIssues")
	listIssues(t, s, openSeven)
	api.checkCalls(t)
}

func TestKeptListWithinTTLIsFresh(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	api := srv.api(t)
	s := New(api, WithStore(store))
	p, err := s.List(t.Context(), openSeven)
	if err != nil || p.Stale || !equalNumbers(numbers(p), 7) {
		t.Fatalf("List in a new session = %+v, %v; want the kept page, fresh", p, err)
	}
	readIssue(t, s)
	api.checkCalls(t)
}

func TestFreshList(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	s := New(srv.api(t), WithStore(store))
	if s.FreshList(openSeven) {
		t.Fatal("FreshList before any read = true, want false")
	}
	listIssues(t, s, openSeven)
	if !s.FreshList(openSeven) {
		t.Error("FreshList after List = false, want true")
	}
	if !s.FreshList(ListQuery{Repo: repo, State: core.FilterOpen, PageSize: 30}) {
		t.Error("FreshList of the same query spelled out = false, want true")
	}
	if s.FreshList(ListQuery{Repo: repo, State: core.FilterClosed}) {
		t.Error("FreshList of another state = true, want false")
	}
	s.Invalidate(repo)
	if s.FreshList(openSeven) {
		t.Error("FreshList after Invalidate = true, want false")
	}

	// A page an earlier session kept within the TTL is fresh.
	if next := New(srv.api(t), WithStore(store)); !next.FreshList(openSeven) {
		t.Error("FreshList of a page kept within the TTL = false, want true")
	}
}

func TestFreshListOfPageKeptLongAgo(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	api := srv.api(t)
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	if s.FreshList(openSeven) {
		t.Fatal("FreshList of a page kept an hour ago = true, want false")
	}
	// A read ahead revalidates it rather than serving the kept page.
	p, err := s.List(t.Context(), openSeven.again())
	if err != nil || p.Stale || !equalNumbers(numbers(p), 7) {
		t.Fatalf("List = %+v, %v; want the page revalidated", p, err)
	}
	api.checkCalls(t, "ListIssues")
}

func TestKeptListChanged(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)
	later := epoch.Add(time.Hour)
	srv.set(later, thread(3))

	s := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
	listIssues(t, s, openSeven)
	p, err := s.List(t.Context(), openSeven.again())
	if err != nil || p.Stale || !p.Items[0].UpdatedAt.Equal(later) {
		t.Fatalf("List after revalidating = %+v, %v; want GitHub's newer page", p, err)
	}
	// The newer page is kept for the session after.
	next := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
	if p, _ := next.List(t.Context(), openSeven); !p.Stale || !p.Items[0].UpdatedAt.Equal(later) {
		t.Errorf("List in the next session = %+v, want the newer page kept", p)
	}
}

func TestKeptIssueIsCurrentAfterRestart(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	api := srv.api(t)
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	listIssues(t, s, openSeven)
	relistIssues(t, s, openSeven)
	api.checkCalls(t, "ListIssues")
	// The list vouches for the kept issue and comments, so they cost
	// nothing, not even a 304.
	it, err := s.Get(t.Context(), repo, 7)
	if err != nil || !it.UpdatedAt.Equal(epoch) {
		t.Fatalf("Get = %+v, %v; want the kept issue", it, err)
	}
	c, err := s.Comments(t.Context(), sevenComments)
	if err != nil || len(c.Items) != 2 {
		t.Fatalf("Comments = %+v, %v; want the kept page", c, err)
	}
	api.checkCalls(t)
	if !current(s, sevenComments) {
		t.Error("Current = false, want true once read from the store")
	}
}

func TestKeptIssueRevalidatesWhenNewer(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)
	later := epoch.Add(time.Hour)
	srv.set(later, thread(3))

	api := srv.api(t)
	var conds []string
	get := api.getIssue
	api.getIssue = func(n int, cond github.Conditional) (core.Issue, github.Response, error) {
		conds = append(conds, cond.ETag)
		return get(n, cond)
	}
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	listIssues(t, s, openSeven)
	relistIssues(t, s, openSeven)
	readIssue(t, s)
	api.checkCalls(t, "ListIssues", "GetIssue", "ListIssueComments")
	if want := fmt.Sprintf(`"%d-2"`, epoch.Unix()); len(conds) != 1 || conds[0] != want {
		t.Errorf("GetIssue validators = %q, want the kept ETag %s", conds, want)
	}
	if c, _ := s.CachedComments(sevenComments); len(c.Items) != 3 {
		t.Errorf("comments = %d, want the 3 GitHub has now", len(c.Items))
	}
}

func TestKeptIssueWithoutList(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)

	// Without a list to vouch for it, the kept issue is revalidated, for
	// free.
	api := srv.api(t)
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	readIssue(t, s)
	api.checkCalls(t, "GetIssue", "ListIssueComments")
}

var errDial = fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("connection refused")})

func TestKeptOffline(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback bool
	}{
		{"unreachable", errDial, true},
		{"server error", &github.Error{StatusCode: 502}, true},
		{"rate limited", fmt.Errorf("list: %w", &core.RateLimitError{Reset: epoch}), true},
		{"not found", &github.Error{StatusCode: 404}, false},
		{"unauthorized", &github.Error{StatusCode: 401}, false},
		{"forbidden", &github.Error{StatusCode: 403}, false},
		// A deleted issue, or a repository that turned its issues off.
		{"gone", &github.Error{StatusCode: 410}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &keptServer{updated: epoch, comments: thread(2)}
			store := openStore(t)
			firstSession(t, srv, store)
			srv.fail(tt.err)

			s := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
			listIssues(t, s, openSeven)
			p, err := s.List(t.Context(), openSeven.again())
			// Without a list to vouch for them, the issue and its
			// comments are asked for.
			other := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
			it, getErr := other.Get(t.Context(), repo, 7)
			c, commentsErr := other.Comments(t.Context(), sevenComments)
			if !tt.fallback {
				if err == nil || getErr == nil || commentsErr == nil {
					t.Errorf("errors = %v, %v, %v; want each read to fail", err, getErr, commentsErr)
				}
				if current(s, sevenComments) {
					t.Error("Current = true after a refusal, want the kept page's vouching forgotten")
				}
				// A refusal drops what was kept.
				srv.fail(nil)
				api := srv.api(t)
				third := New(api, WithStore(cachetest.Aged(store, time.Hour)))
				if p, _ := third.List(t.Context(), openSeven); p.Stale {
					t.Error("List after a refusal = stale, want the kept page gone")
				}
				readIssue(t, third)
				api.checkCalls(t, "ListIssues", "GetIssue", "ListIssueComments")
				return
			}
			limited := errors.Is(tt.err, core.ErrRateLimited)
			if err != nil || p.Offline == limited || p.Limited != limited || len(p.Items) != 1 {
				t.Errorf("List = %+v, %v; want the kept page, offline or limited", p, err)
			}
			if getErr != nil || it.Number != 7 {
				t.Errorf("Get = %+v, %v; want the kept issue", it, getErr)
			}
			if commentsErr != nil || c.Offline == limited || c.Limited != limited || len(c.Items) != 2 {
				t.Errorf("Comments = %+v, %v; want the kept page, offline or limited", c, commentsErr)
			}
		})
	}
}

// TestKeptAnsweredAfterOutage checks that what was served offline is
// served unmarked once GitHub confirms it with a 304, whether a read or
// the revalidator asks.
func TestKeptAnsweredAfterOutage(t *testing.T) {
	for _, revalidated := range []bool{false, true} {
		t.Run(fmt.Sprintf("revalidated=%v", revalidated), func(t *testing.T) {
			srv := &keptServer{updated: epoch, comments: thread(2)}
			store := openStore(t)
			firstSession(t, srv, store)
			srv.fail(errDial)
			s := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
			listIssues(t, s, openSeven)
			p, err := s.List(t.Context(), openSeven.again())
			// Without a list to vouch for them, the comments are asked for.
			other := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
			c, commentsErr := other.Comments(t.Context(), sevenComments)
			if err != nil || !p.Offline || commentsErr != nil || !c.Offline {
				t.Fatalf("reads while offline = %+v, %v and %+v, %v; want both offline", p, err, c, commentsErr)
			}

			srv.fail(nil)
			if revalidated {
				list, _ := s.listTarget(listKey(openSeven.normalize(30)))
				comments, _ := other.commentsTarget(commentsKey(sevenComments.normalize(30)))
				for _, target := range []recheck.Target{list, comments} {
					if res := target.Check(t.Context()); res.Status != revalidate.NotModified {
						t.Fatalf("check = %+v, want not modified", res)
					}
				}
			}
			p, err = s.List(t.Context(), openSeven)
			c, commentsErr = other.Comments(t.Context(), sevenComments)
			if err != nil || p.Offline || p.Stale || commentsErr != nil || c.Offline {
				t.Errorf("reads after a 304 = %+v, %v and %+v, %v; want both unmarked", p, err, c, commentsErr)
			}
		})
	}
}

func TestKeptWithoutStoreIsNotOffline(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	srv.fail(errDial)
	if _, err := New(srv.api(t)).List(t.Context(), openSeven); !errors.Is(err, errDial) {
		t.Errorf("List error = %v, want the network error", err)
	}
}

func TestKeptCommentIsNotKeptUntilConfirmed(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		t.Run(fmt.Sprintf("confirmed=%v", confirm), func(t *testing.T) {
			srv := &keptServer{updated: epoch, comments: thread(2)}
			store := openStore(t)
			api := srv.api(t)
			s := New(api, WithStore(cachetest.Aged(store, time.Hour)), WithViewer("octocat"))
			listIssues(t, s, openSeven)
			readIssue(t, s)

			op := s.Comment(repo, 7, "On it")
			// A revalidation while the comment is pending brings a 304,
			// which must not keep the pending comment.
			s.Invalidate(repo)
			readIssue(t, s)
			if c, _ := s.CachedComments(sevenComments); len(c.Items) != 3 || !IsPending(c.Items[2]) {
				t.Fatalf("comments while pending = %+v, want the pending one shown", c.Items)
			}
			api.comment = func(int, string) (core.Comment, error) {
				if !confirm {
					return core.Comment{}, core.ErrConflict
				}
				posted := comment(3)
				srv.set(epoch.Add(time.Minute), thread(3))
				return posted, nil
			}
			_ = op.Do(t.Context())

			next := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
			if confirm {
				// The next read after the change brings GitHub's page.
				listIssues(t, next, openSeven)
				relistIssues(t, next, openSeven)
			}
			c, err := next.Comments(t.Context(), sevenComments)
			if err != nil {
				t.Fatalf("Comments in the next session: %v", err)
			}
			for _, cm := range c.Items {
				if IsPending(cm) {
					t.Errorf("next session shows %+v, want no pending comment kept", cm)
				}
			}
			if want := map[bool]int{false: 2, true: 3}[confirm]; len(c.Items) != want {
				t.Errorf("next session shows %d comments, want %d", len(c.Items), want)
			}
		})
	}
}

func TestKeptStateChangeAfterConfirm(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	api := srv.api(t)
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	readIssue(t, s)
	closed := issue(7)
	closed.State, closed.UpdatedAt = core.StateClosed, epoch.Add(time.Minute)
	api.setState = func(int, core.State) (core.Issue, error) { return closed, nil }
	if err := s.Close(repo, 7).Do(t.Context()); err != nil {
		t.Fatal(err)
	}
	e, ok := New(api, WithStore(cachetest.Aged(store, time.Hour))).keptIssues.Load(issueKey(repo, 7))
	if !ok || e.Value.State != core.StateClosed || e.ETag != "" {
		t.Errorf("kept issue = %+v, %v; want GitHub's closed issue without validators", e, ok)
	}
}

func TestKeptRollbackLeavesStoreAlone(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	api := srv.api(t)
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	listIssues(t, s, openSeven)
	readIssue(t, s)
	api.setState = func(int, core.State) (core.Issue, error) { return core.Issue{}, core.ErrConflict }
	if err := s.Close(repo, 7).Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the conflict")
	}
	next := New(srv.api(t), WithStore(cachetest.Aged(store, time.Hour)))
	p, _ := next.List(t.Context(), openSeven)
	e, _ := next.keptIssues.Load(issueKey(repo, 7))
	if p.Items[0].State != core.StateOpen || e.Value.State != core.StateOpen {
		t.Errorf("kept state = %s and %s, want open as GitHub has it", p.Items[0].State, e.Value.State)
	}
}

func TestKeptCorruptEntryIsAMiss(t *testing.T) {
	srv := &keptServer{updated: epoch, comments: thread(2)}
	store := openStore(t)
	firstSession(t, srv, store)
	err := filepath.WalkDir(store.Dir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.WriteFile(p, []byte("garbage"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}

	api := srv.api(t)
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.List(t.Context(), openSeven)
	if err != nil || p.Stale || len(p.Items) != 1 {
		t.Errorf("List = %+v, %v; want a page read from GitHub", p, err)
	}
	api.checkCalls(t, "ListIssues")
}

func equalNumbers(got []int, want ...int) bool {
	return fmt.Sprint(got) == fmt.Sprint(want)
}

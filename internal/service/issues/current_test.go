package issues

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// versionedAPI is a fake GitHub whose issue 7 was last updated at the
// time that set sets. Every read answers in full, never 304, so that each
// request shows in the calls.
func versionedAPI(t *testing.T) (api *fakeAPI, set func(time.Time)) {
	t.Helper()
	var (
		mu      sync.Mutex
		updated = epoch
	)
	current := func() core.Issue {
		mu.Lock()
		defer mu.Unlock()
		it := issue(7)
		it.UpdatedAt = updated
		return it
	}
	api = &fakeAPI{
		t: t,
		listIssues: func(core.StateFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
			return core.Page[core.Issue]{Items: []core.Issue{current()}}, ok(`"l"`), nil
		},
		getIssue: func(int, github.Conditional) (core.Issue, github.Response, error) {
			return current(), ok(`"i"`), nil
		},
		listComments: func(_ int, cursor string, perPage int, _ github.Conditional) (core.Page[core.Comment], github.Response, error) {
			return commentPage(t, thread(2), cursor, perPage), ok(`"c"`), nil
		},
	}
	set = func(t time.Time) {
		mu.Lock()
		defer mu.Unlock()
		updated = t
	}
	return api, set
}

var sevenComments = CommentsQuery{Repo: repo, Number: 7}

func readIssue(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.Comments(t.Context(), sevenComments); err != nil {
		t.Fatalf("Comments: %v", err)
	}
}

func listIssues(t *testing.T, s *Service, q ListQuery) {
	t.Helper()
	if _, err := s.List(t.Context(), q); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestCurrentIssueOutlivesTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api, _ := versionedAPI(t)
		s := New(api, WithTTL(time.Minute))
		listIssues(t, s, ListQuery{Repo: repo})
		readIssue(t, s)
		api.checkCalls(t, "ListIssues", "GetIssue", "ListIssueComments")

		time.Sleep(time.Hour)
		if !s.Current(sevenComments) {
			t.Error("Current = false past the TTL, want true: the list vouches for 7")
		}
		// Not even a conditional request.
		readIssue(t, s)
		api.checkCalls(t)
	})
}

func TestNewerIssueRefetches(t *testing.T) {
	api, set := versionedAPI(t)
	s := New(api)
	listIssues(t, s, ListQuery{Repo: repo})
	readIssue(t, s)
	api.called()

	// Another list page, well within the TTL, shows that 7 changed.
	set(epoch.Add(time.Minute))
	listIssues(t, s, ListQuery{Repo: repo, PageSize: 10})
	if s.Current(sevenComments) {
		t.Error("Current = true after 7 changed, want false")
	}
	readIssue(t, s)
	api.checkCalls(t, "ListIssues", "GetIssue", "ListIssueComments")
	if it, _ := s.CachedGet(repo, 7); !it.UpdatedAt.Equal(epoch.Add(time.Minute)) {
		t.Errorf("cached issue updated at %v, want the new version", it.UpdatedAt)
	}
	readIssue(t, s)
	api.checkCalls(t)
}

func TestUnlistedIssueFollowsTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Opened from the search, no list vouches for 7.
		api, _ := versionedAPI(t)
		s := New(api, WithTTL(time.Minute))
		readIssue(t, s)
		readIssue(t, s)
		api.checkCalls(t, "GetIssue", "ListIssueComments")

		time.Sleep(2 * time.Minute)
		if s.Current(sevenComments) {
			t.Error("Current = true past the TTL without a list")
		}
		readIssue(t, s)
		api.checkCalls(t, "GetIssue", "ListIssueComments")
	})
}

func TestNotModifiedCommentsTakeTheVersion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api, _ := versionedAPI(t)
		api.listComments = func(_ int, cursor string, perPage int, c github.Conditional) (core.Page[core.Comment], github.Response, error) {
			if c.ETag != "" {
				return core.Page[core.Comment]{}, notModified, nil
			}
			return commentPage(t, thread(2), cursor, perPage), ok(`"c"`), nil
		}
		s := New(api, WithTTL(time.Minute))
		// Read before any list, the page has no version.
		readIssue(t, s)
		listIssues(t, s, ListQuery{Repo: repo})
		time.Sleep(2 * time.Minute)
		// The issue is current, the page asks and GitHub confirms it.
		readIssue(t, s)
		api.checkCalls(t, "GetIssue", "ListIssueComments", "ListIssues", "ListIssueComments")

		time.Sleep(2 * time.Minute)
		readIssue(t, s)
		api.checkCalls(t)
	})
}

func TestIssueChangeForgetsVersion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api, _ := versionedAPI(t)
		api.setState = func(int, core.State) (core.Issue, error) {
			it := issue(7)
			it.State = core.StateClosed
			return it, nil
		}
		s := New(api, WithTTL(time.Minute))
		listIssues(t, s, ListQuery{Repo: repo})
		readIssue(t, s)
		if err := s.Close(repo, 7).Do(t.Context()); err != nil {
			t.Fatalf("Close: %v", err)
		}
		api.called()

		time.Sleep(2 * time.Minute)
		readIssue(t, s)
		api.checkCalls(t, "GetIssue", "ListIssueComments")
	})
}

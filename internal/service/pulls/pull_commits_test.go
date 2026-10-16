package pulls

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func commitsRead() *fakeAPI {
	return &fakeAPI{
		viewer: "me",
		pullCommits: core.PullCommits{
			Commits:    []core.PullCommit{{OID: "c1", Checks: core.CheckPending}},
			LastReview: &core.ViewerReview{Commit: "c1"},
		},
	}
}

func TestPullCommitsAreCachedByHead(t *testing.T) {
	api := commitsRead()
	s := New(api)
	q := PullCommitsQuery{Repo: repo, Number: 1, Head: "AAA"}
	if _, ok := s.CachedPullCommits(q); ok {
		t.Fatal("commits are cached before they were read")
	}
	for range 2 {
		got, err := s.PullCommits(t.Context(), q)
		if err != nil || !reflect.DeepEqual(got, api.pullCommits) {
			t.Fatalf("PullCommits = %+v, %v; want the commits", got, err)
		}
	}
	if n := api.count("pullcommits"); n != 1 {
		t.Errorf("reads = %d, want 1: the second was cached", n)
	}
	if !reflect.DeepEqual(api.viewers, []string{"me"}) {
		t.Errorf("viewers = %q, want the signed-in login", api.viewers)
	}
	if _, ok := s.CachedPullCommits(PullCommitsQuery{Repo: repo, Number: 1, Head: "aaa"}); !ok {
		t.Error("the head's case changes the key")
	}

	// A push is a new head, which misses.
	if _, ok := s.CachedPullCommits(PullCommitsQuery{Repo: repo, Number: 1, Head: "bbb"}); ok {
		t.Error("another head is cached")
	}
	if _, err := s.PullCommits(t.Context(), PullCommitsQuery{Repo: repo, Number: 1, Head: "bbb"}); err != nil {
		t.Fatal(err)
	}
	if n := api.count("pullcommits"); n != 2 {
		t.Errorf("reads = %d, want 2: another head reads again", n)
	}
	// So does another pull request.
	if _, err := s.PullCommits(t.Context(), PullCommitsQuery{Repo: repo, Number: 2, Head: "aaa"}); err != nil {
		t.Fatal(err)
	}
	if n := api.count("pullcommits"); n != 3 {
		t.Errorf("reads = %d, want 3", n)
	}
	// The login is read once.
	if n := api.count("viewer"); n != 1 {
		t.Errorf("login reads = %d, want 1", n)
	}
}

func TestPullCommitsReadAgainAfterARefresh(t *testing.T) {
	q := PullCommitsQuery{Repo: repo, Number: 1, Head: "aaa"}
	tests := []struct {
		name    string
		refresh func(t *testing.T, s *Service, api *fakeAPI)
	}{
		{"a refresh of the repository", func(_ *testing.T, s *Service, _ *fakeAPI) { s.Invalidate(repo) }},
		{"a change a probe found", func(_ *testing.T, s *Service, _ *fakeAPI) { s.invalidate(repo) }},
		{"a revalidated detail", func(t *testing.T, s *Service, api *fakeAPI) {
			t.Helper()
			api.get = detailWithThreads
			if _, err := s.Revalidate(t.Context(), repo, 1); err != nil {
				t.Fatal(err)
			}
		}},
		{"a submitted review", func(_ *testing.T, s *Service, _ *fakeAPI) { s.reviewSubmitted(repo, 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := commitsRead()
			s := New(api)
			if _, err := s.PullCommits(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			tt.refresh(t, s, api)
			if _, ok := s.CachedPullCommits(q); !ok {
				t.Error("the stale commits are no longer served by the cached read")
			}
			if _, err := s.PullCommits(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			if n := api.count("pullcommits"); n != 2 {
				t.Errorf("reads = %d, want 2", n)
			}
		})
	}
}

func TestPullCommitsOfAnotherPullStayFresh(t *testing.T) {
	api := commitsRead()
	s := New(api)
	other := PullCommitsQuery{Repo: repo, Number: 2, Head: "aaa"}
	if _, err := s.PullCommits(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	api.get = detailWithThreads
	if _, err := s.Revalidate(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PullCommits(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	if n := api.count("pullcommits"); n != 1 {
		t.Errorf("reads = %d, want 1: the revalidation of pull 1 is not pull 2's", n)
	}
}

func TestPullCommitsFailures(t *testing.T) {
	s := New(commitsRead())
	if _, err := s.PullCommits(t.Context(), PullCommitsQuery{Repo: repo, Number: 1}); !errors.Is(err, errNoHead) {
		t.Errorf("err = %v, want errNoHead", err)
	}

	boom := errors.New("boom")
	api := commitsRead()
	api.pullCommitsErr = boom
	s = New(api)
	q := PullCommitsQuery{Repo: repo, Number: 1, Head: "aaa"}
	if _, err := s.PullCommits(t.Context(), q); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the failure", err)
	}
	if _, ok := s.CachedPullCommits(q); ok {
		t.Error("a failure was cached")
	}

	api = commitsRead()
	api.viewerErr = boom
	s = New(api)
	if _, err := s.PullCommits(t.Context(), q); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the login's failure", err)
	}
	api.viewerErr = nil
	if _, err := s.PullCommits(t.Context(), q); err != nil {
		t.Errorf("err = %v, want the read to try the login again", err)
	}
}

package facets

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

var repo = core.RepoRef{Owner: "octo-org", Name: "hello"}

// fakeAPI answers with fixed values and records the calls, and the
// conditions of the conditional ones.
type fakeAPI struct {
	mu    sync.Mutex
	calls []string
	conds []github.Conditional
	// notModified answers the conditional reads with a 304.
	notModified bool
	err         error
}

func (f *fakeAPI) record(call string, cond github.Conditional) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	f.conds = append(f.conds, cond)
}

func (f *fakeAPI) response(cond github.Conditional) github.Response {
	if f.notModified && cond.ETag != "" {
		return github.Response{NotModified: true}
	}
	return github.Response{ETag: `"e1"`}
}

func (f *fakeAPI) ListLabels(_ context.Context, r core.RepoRef, cond github.Conditional) ([]core.Label, github.Response, error) {
	f.record("labels "+r.String(), cond)
	return []core.Label{{Name: "bug"}}, f.response(cond), f.err
}

func (f *fakeAPI) ListMilestones(_ context.Context, r core.RepoRef, cond github.Conditional) ([]core.Milestone, github.Response, error) {
	f.record("milestones "+r.String(), cond)
	return []core.Milestone{{Number: 2, Title: "v2"}}, f.response(cond), f.err
}

func (f *fakeAPI) RepoUsers(_ context.Context, r core.RepoRef, query string, first int) ([]core.User, error) {
	f.record("people "+r.String()+" "+query, github.Conditional{})
	if first != peopleLimit {
		return nil, errors.New("unexpected page size")
	}
	return []core.User{{Login: "octocat"}}, f.err
}

func TestReadsAreCached(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	if _, ok := s.CachedLabels(repo); ok {
		t.Error("CachedLabels reported labels before a read")
	}
	for range 2 {
		if l, err := s.Labels(t.Context(), repo); err != nil || len(l) != 1 {
			t.Fatalf("Labels = %v, %v", l, err)
		}
		if m, err := s.Milestones(t.Context(), repo); err != nil || m[0].Title != "v2" {
			t.Fatalf("Milestones = %v, %v", m, err)
		}
		if p, err := s.People(t.Context(), PeopleQuery{Repo: repo, Text: "oct"}); err != nil || p[0].Login != "octocat" {
			t.Fatalf("People = %v, %v", p, err)
		}
	}
	// The same repository in other casing, and the same text in other
	// casing, are the same reads.
	upper := core.RepoRef{Owner: "Octo-Org", Name: "Hello"}
	if _, err := s.People(t.Context(), PeopleQuery{Repo: upper, Text: " OCT "}); err != nil {
		t.Fatal(err)
	}
	want := []string{"labels octo-org/hello", "milestones octo-org/hello", "people octo-org/hello oct"}
	if !slices.Equal(api.calls, want) {
		t.Errorf("calls = %q, want each read once: %q", api.calls, want)
	}
	if l, ok := s.CachedLabels(repo); !ok || l[0].Name != "bug" {
		t.Errorf("CachedLabels = %v, %v; want the labels read", l, ok)
	}
	if m, ok := s.CachedMilestones(repo); !ok || len(m) != 1 {
		t.Errorf("CachedMilestones = %v, %v; want the milestones read", m, ok)
	}
	if p, ok := s.CachedPeople(PeopleQuery{Repo: repo, Text: "oct"}); !ok || len(p) != 1 {
		t.Errorf("CachedPeople = %v, %v; want the people read", p, ok)
	}
}

func TestStaleLabelsAreRevalidated(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{notModified: true}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Labels(t.Context(), repo); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Minute)
		l, err := s.Labels(t.Context(), repo)
		if err != nil || len(l) != 1 {
			t.Fatalf("Labels after the TTL = %v, %v; want the cached ones", l, err)
		}
		if len(api.conds) != 2 || api.conds[1].ETag != `"e1"` {
			t.Errorf("conditions = %+v, want the second read conditional on the ETag", api.conds)
		}
	})
}

func TestErrorsAreWrappedAndNotCached(t *testing.T) {
	api := &fakeAPI{err: core.ErrNotFound}
	s := New(api)
	if _, err := s.Labels(t.Context(), repo); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Labels error = %v, want ErrNotFound", err)
	}
	if _, err := s.Milestones(t.Context(), repo); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Milestones error = %v, want ErrNotFound", err)
	}
	if _, err := s.People(t.Context(), PeopleQuery{Repo: repo}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("People error = %v, want ErrNotFound", err)
	}
	if _, ok := s.CachedLabels(repo); ok {
		t.Error("a failed read was cached")
	}
}

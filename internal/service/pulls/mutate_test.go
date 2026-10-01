package pulls

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

var (
	openFirst  = ListQuery{Repo: repo, State: core.StateOpen}
	openSecond = ListQuery{Repo: repo, State: core.StateOpen, Cursor: "c1"}
	clock      = time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	serverTime = clock.Add(5 * time.Second)
)

type mutation struct {
	name string
	// seed puts #1 in the state the mutation starts from.
	seed func(*core.PullRequest)
	run  func(s *Service, number int) *optimistic.Op
	// method is the fake API method the mutation calls, and how is the merge
	// method it passes.
	method string
	how    core.MergeMethod
	// changed reports whether pr shows the change.
	changed func(pr core.PullRequest) bool
}

var mutations = []mutation{
	{
		name:    "merge",
		seed:    func(*core.PullRequest) {},
		run:     func(s *Service, n int) *optimistic.Op { return s.Merge(repo, n, core.MergeRebase, "9f1c2e4") },
		method:  "merge",
		how:     core.MergeRebase,
		changed: func(pr core.PullRequest) bool { return pr.State == core.StateMerged && pr.MergedAt.Equal(clock) },
	},
	{
		name:    "close",
		seed:    func(*core.PullRequest) {},
		run:     func(s *Service, n int) *optimistic.Op { return s.Close(repo, n) },
		method:  "close",
		changed: func(pr core.PullRequest) bool { return pr.State == core.StateClosed },
	},
	{
		name:    "reopen",
		seed:    func(pr *core.PullRequest) { pr.State = core.StateClosed },
		run:     func(s *Service, n int) *optimistic.Op { return s.Reopen(repo, n) },
		method:  "reopen",
		changed: func(pr core.PullRequest) bool { return pr.State == core.StateOpen },
	},
	{
		name:    "mark ready",
		seed:    func(pr *core.PullRequest) { pr.Draft = true },
		run:     func(s *Service, n int) *optimistic.Op { return s.MarkReady(repo, n) },
		method:  "ready",
		changed: func(pr core.PullRequest) bool { return !pr.Draft },
	},
	{
		name:    "convert to draft",
		seed:    func(*core.PullRequest) {},
		run:     func(s *Service, n int) *optimistic.Op { return s.ConvertToDraft(repo, n) },
		method:  "draft",
		changed: func(pr core.PullRequest) bool { return pr.Draft },
	},
}

// seeded returns a service that has cached both open list pages and the
// detail of #1, with #1 as seed leaves it.
func seeded(t *testing.T, api *fakeAPI, seed func(*core.PullRequest)) *Service {
	t.Helper()
	api.list = func(ctx context.Context, r core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
		p, err := listing(ctx, r, state, cursor, first)
		for i := range p.Items {
			if p.Items[i].Number == 1 {
				seed(&p.Items[i])
			}
		}
		return p, err
	}
	api.get = func(ctx context.Context, r core.RepoRef, number int) (core.PullRequestDetail, error) {
		d, err := detail(ctx, r, number)
		d.Body = "The body."
		seed(&d.PullRequest)
		return d, err
	}
	s := New(api)
	s.now = func() time.Time { return clock }
	for _, q := range []ListQuery{openFirst, openSecond} {
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatalf("Get: %v", err)
	}
	return s
}

// snapshot is what the tui can render without I/O.
type snapshot struct {
	first, second core.Page[core.PullRequest]
	detail        core.PullRequestDetail
}

func take(t *testing.T, s *Service) snapshot {
	t.Helper()
	first, ok1 := s.CachedList(openFirst)
	second, ok2 := s.CachedList(openSecond)
	d, ok3 := s.CachedGet(repo, 1)
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("cached = %v, %v, %v; want every entry", ok1, ok2, ok3)
	}
	return snapshot{first, second, d}
}

func TestMutationShownBeforeDo(t *testing.T) {
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			s := seeded(t, &fakeAPI{}, m.seed)
			before := take(t, s)
			if m.changed(before.first.Items[0]) {
				t.Fatal("seed already shows the change")
			}

			m.run(s, 1)
			after := take(t, s)
			if !m.changed(after.first.Items[0]) {
				t.Errorf("list shows %+v, want the change", after.first.Items[0])
			}
			if !m.changed(after.detail.PullRequest) {
				t.Errorf("detail shows %+v, want the change", after.detail.PullRequest)
			}
			if after.detail.Body != "The body." || after.detail.CheckCounts.Total() != 1 {
				t.Errorf("detail = %+v, want its body and checks kept", after.detail)
			}
			if !reflect.DeepEqual(after.first.Items[1], before.first.Items[1]) || !reflect.DeepEqual(after.second, before.second) {
				t.Error("the change touched other pull requests")
			}
			if m.changed(before.first.Items[0]) || m.changed(before.detail.PullRequest) {
				t.Error("the change modified the cached values in place")
			}
		})
	}
}

func TestMutationFailureRollsBack(t *testing.T) {
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			api := &fakeAPI{mutate: func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
				return core.PullRequest{}, errors.Join(errors.New("github: 422"), core.ErrConflict)
			}}
			s := seeded(t, api, m.seed)
			before := take(t, s)

			op := m.run(s, 1)
			err := op.Do(t.Context())
			if !errors.Is(err, core.ErrConflict) {
				t.Errorf("Do error = %v, want ErrConflict", err)
			}
			if err != nil && !strings.Contains(err.Error(), m.name+" pull eggzec/gh-tui#1") {
				t.Errorf("error %q lacks context", err)
			}
			if after := take(t, s); !reflect.DeepEqual(after, before) {
				t.Errorf("after rollback =\n%+v\nwant\n%+v", after, before)
			}
			if n := api.count(m.method); n != 1 {
				t.Errorf("%s called %d times, want 1", m.method, n)
			}
		})
	}
}

func TestMutationSuccessReconciles(t *testing.T) {
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			var server core.PullRequest
			api := &fakeAPI{}
			api.mutate = func(_ context.Context, method, id string, how core.MergeMethod) (core.PullRequest, error) {
				if method != m.method || id != "PR_1" || how != m.how {
					t.Errorf("mutation = %s(%q, %q), want %s(PR_1, %q)", method, id, how, m.method, m.how)
				}
				return server, nil
			}
			s := seeded(t, api, m.seed)
			op := m.run(s, 1)
			// The server's answer differs from the optimistic change, so the
			// test can tell which one is stored.
			server = take(t, s).first.Items[0]
			server.Title = "Title from the server"
			server.UpdatedAt = serverTime
			if m.method == "merge" {
				server.MergedAt = serverTime
			}

			if err := op.Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			after := take(t, s)
			if !reflect.DeepEqual(after.first.Items[0], server) {
				t.Errorf("list shows\n%+v\nwant the server's\n%+v", after.first.Items[0], server)
			}
			want := server
			want.Body = "The body."
			if !reflect.DeepEqual(after.detail.PullRequest, want) || after.detail.CheckCounts.Total() != 1 {
				t.Errorf("detail shows\n%+v\nwant the server's with the body and checks kept\n%+v", after.detail, want)
			}
			if n := api.count("id"); n != 0 {
				t.Errorf("looked up the node ID %d times, want 0 as it was cached", n)
			}

			// The pages may no longer match their filter, so they are
			// fetched again, but the detail is current.
			if _, err := s.List(t.Context(), openFirst); err != nil {
				t.Fatalf("List: %v", err)
			}
			if n := api.count("list"); n != 3 {
				t.Errorf("list called %d times, want 3: pages must be stale after a change", n)
			}
			if _, err := s.Get(t.Context(), repo, 1); err != nil {
				t.Fatalf("Get: %v", err)
			}
			if n := api.count("get"); n != 1 {
				t.Errorf("get called %d times, want 1: the reconciled detail is fresh", n)
			}
		})
	}
}

func TestMutationUncachedSends(t *testing.T) {
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			api := &fakeAPI{
				id: func(_ context.Context, r core.RepoRef, number int) (string, error) {
					if r != repo || number != 7 {
						t.Errorf("looked up %s#%d, want %s#7", r, number, repo)
					}
					return "PR_node7", nil
				},
				mutate: func(_ context.Context, _, id string, _ core.MergeMethod) (core.PullRequest, error) {
					if id != "PR_node7" {
						t.Errorf("mutation got id %q, want PR_node7", id)
					}
					return openPull(7), nil
				},
			}
			s := New(api)
			if err := m.run(s, 7).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if api.count("id") != 1 || api.count(m.method) != 1 {
				t.Errorf("%d lookups and %d calls of %s, want one each", api.count("id"), api.count(m.method), m.method)
			}
			if _, ok := s.CachedGet(repo, 7); ok {
				t.Error("the change cached a detail that was never fetched")
			}
		})
	}
}

func TestMutationLookupFails(t *testing.T) {
	api := &fakeAPI{id: func(context.Context, core.RepoRef, int) (string, error) {
		return "", core.ErrNotFound
	}}
	s := New(api)
	err := s.Close(repo, 7).Do(t.Context())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Do error = %v, want ErrNotFound", err)
	}
	if n := api.count("close"); n != 0 {
		t.Errorf("close called %d times, want 0 without a node ID", n)
	}
}

// A merge sends the head it was confirmed for, which pins it there.
func TestMergeSendsHead(t *testing.T) {
	for _, head := range []string{"9f1c2e4", ""} {
		api := &fakeAPI{mutate: func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
			return core.PullRequest{Number: 1, State: core.StateMerged}, nil
		}}
		s := seeded(t, api, func(*core.PullRequest) {})
		if err := s.Merge(repo, 1, core.MergeSquash, head).Do(t.Context()); err != nil {
			t.Fatalf("merge: %v", err)
		}
		api.mu.Lock()
		got := api.heads
		api.mu.Unlock()
		if len(got) != 1 || got[0] != head {
			t.Errorf("merge sent heads %q, want [%q]", got, head)
		}
	}
}

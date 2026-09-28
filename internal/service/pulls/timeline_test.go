package pulls

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// page is a timeline page reduced to what the tests compare: the IDs of its
// items and its Next.
type page struct {
	ids  []string
	next string
}

// pageRequest is a timeline page as the fake API was asked for it.
type pageRequest struct {
	repo   core.RepoRef
	number int
	cursor string
	first  int
}

// serveFunc answers a timeline request of the fake API.
type serveFunc func(pageRequest) (page, error)

// timelineRead drives one of the paged reads, comments or reviews, so that
// both run through the same tests.
type timelineRead struct {
	name string
	// method is the fake API method that backs the read.
	method string
	serve  func(api *fakeAPI, f serveFunc)
	fetch  func(ctx context.Context, s *Service, q pageRequest) (page, error)
	cached func(s *Service, q pageRequest) (page, bool)
	// invalidateTag marks the read's cached pages under tag stale.
	invalidateTag func(s *Service, tag string)
}

func toPage[T any](p core.Page[T], id func(T) string) page {
	out := page{next: p.Next}
	for _, v := range p.Items {
		out.ids = append(out.ids, id(v))
	}
	return out
}

func commentID(c core.Comment) string { return c.ID }
func reviewID(r core.Review) string   { return r.ID }

var timelineReads = []timelineRead{
	{
		name:   "comments",
		method: "comments",
		serve: func(api *fakeAPI, f serveFunc) {
			api.comments = func(_ context.Context, r core.RepoRef, n int, cursor string, first int) (core.Page[core.Comment], error) {
				p, err := f(pageRequest{r, n, cursor, first})
				out := core.Page[core.Comment]{Next: p.next}
				for _, id := range p.ids {
					out.Items = append(out.Items, core.Comment{ID: id})
				}
				return out, err
			}
		},
		fetch: func(ctx context.Context, s *Service, q pageRequest) (page, error) {
			p, err := s.Comments(ctx, CommentsQuery{Repo: q.repo, Number: q.number, Cursor: q.cursor, PageSize: q.first})
			return toPage(p, commentID), err
		},
		cached: func(s *Service, q pageRequest) (page, bool) {
			p, ok := s.CachedComments(CommentsQuery{Repo: q.repo, Number: q.number, Cursor: q.cursor, PageSize: q.first})
			return toPage(p, commentID), ok
		},
		invalidateTag: func(s *Service, tag string) { s.comments.InvalidateTag(tag) },
	},
	{
		name:   "reviews",
		method: "reviews",
		serve: func(api *fakeAPI, f serveFunc) {
			api.reviews = func(_ context.Context, r core.RepoRef, n int, cursor string, first int) (core.Page[core.Review], error) {
				p, err := f(pageRequest{r, n, cursor, first})
				out := core.Page[core.Review]{Next: p.next}
				for _, id := range p.ids {
					out.Items = append(out.Items, core.Review{ID: id})
				}
				return out, err
			}
		},
		fetch: func(ctx context.Context, s *Service, q pageRequest) (page, error) {
			p, err := s.Reviews(ctx, ReviewsQuery{Repo: q.repo, Number: q.number, Cursor: q.cursor, PageSize: q.first})
			return toPage(p, reviewID), err
		},
		cached: func(s *Service, q pageRequest) (page, bool) {
			p, ok := s.CachedReviews(ReviewsQuery{Repo: q.repo, Number: q.number, Cursor: q.cursor, PageSize: q.first})
			return toPage(p, reviewID), ok
		},
		invalidateTag: func(s *Service, tag string) { s.reviews.InvalidateTag(tag) },
	},
}

// thread serves a thread of three items in pages: the first page holds a
// and b and continues at cursor "c1", which holds c.
func thread(q pageRequest) (page, error) {
	if q.cursor == "c1" {
		return page{ids: []string{"c"}}, nil
	}
	return page{ids: []string{"a", "b"}, next: "c1"}, nil
}

func TestTimelineFreshHitMakesNoCall(t *testing.T) {
	for _, r := range timelineReads {
		t.Run(r.name, func(t *testing.T) {
			api := &fakeAPI{}
			r.serve(api, thread)
			s := New(api)
			q := pageRequest{repo: repo, number: 1}

			if _, ok := r.cached(s, q); ok {
				t.Error("cached reported a page before any fetch")
			}
			for range 2 {
				p, err := r.fetch(t.Context(), s, q)
				if err != nil {
					t.Fatalf("fetch: %v", err)
				}
				if want := (page{[]string{"a", "b"}, "c1"}); !reflect.DeepEqual(p, want) {
					t.Errorf("page = %+v, want %+v", p, want)
				}
			}
			if n := api.count(r.method); n != 1 {
				t.Errorf("API called %d times, want 1", n)
			}
			if p, ok := r.cached(s, q); !ok || p.next != "c1" {
				t.Errorf("cached = %+v, %v; want the fetched page", p, ok)
			}
		})
	}
}

func TestTimelineKeysByPage(t *testing.T) {
	for _, r := range timelineReads {
		t.Run(r.name, func(t *testing.T) {
			var (
				mu    sync.Mutex
				calls []pageRequest
			)
			api := &fakeAPI{}
			r.serve(api, func(q pageRequest) (page, error) {
				mu.Lock()
				calls = append(calls, q)
				mu.Unlock()
				return thread(q)
			})
			s := New(api)
			fetch := func(q pageRequest) page {
				t.Helper()
				p, err := r.fetch(t.Context(), s, q)
				if err != nil {
					t.Fatalf("fetch: %v", err)
				}
				return p
			}

			first := fetch(pageRequest{repo: repo, number: 1})
			second := fetch(pageRequest{repo: repo, number: 1, cursor: first.next})
			if want := (page{ids: []string{"c"}}); !reflect.DeepEqual(second, want) {
				t.Errorf("second page = %+v, want the last page with c", second)
			}
			fetch(pageRequest{repo: repo, number: 2})
			// The same page with other casing is the same repository.
			fetch(pageRequest{repo: core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}, number: 1})
			// Zero and negative sizes mean the default, and sizes above
			// GitHub's maximum are clamped to it, so each pair shares a page.
			for _, size := range []int{30, -1, 10, 10, 500, maxPageSize} {
				fetch(pageRequest{repo: repo, number: 1, first: size})
			}
			// Each page of a size is its own entry too.
			fetch(pageRequest{repo: repo, number: 1, cursor: "c1", first: 10})

			want := []pageRequest{
				{repo, 1, "", 30},
				{repo, 1, "c1", 30},
				{repo, 2, "", 30},
				{repo, 1, "", 10},
				{repo, 1, "", maxPageSize},
				{repo, 1, "c1", 10},
			}
			if !reflect.DeepEqual(calls, want) {
				t.Errorf("calls =\n%+v\nwant\n%+v", calls, want)
			}
		})
	}
}

func TestTimelineTaggedByRepo(t *testing.T) {
	for _, r := range timelineReads {
		t.Run(r.name, func(t *testing.T) {
			api := &fakeAPI{}
			r.serve(api, thread)
			s := New(api)
			pages := []pageRequest{{repo: repo, number: 1}, {repo: repo, number: 1, cursor: "c1"}, {repo: repo, number: 2}}
			for _, q := range pages {
				if _, err := r.fetch(t.Context(), s, q); err != nil {
					t.Fatalf("fetch: %v", err)
				}
			}

			r.invalidateTag(s, repoTag(core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}))
			for _, q := range pages {
				if _, ok := r.cached(s, q); !ok {
					t.Errorf("page %+v was dropped, want it kept stale", q)
				}
				if _, err := r.fetch(t.Context(), s, q); err != nil {
					t.Fatalf("fetch: %v", err)
				}
			}
			if n := api.count(r.method); n != 2*len(pages) {
				t.Errorf("API called %d times, want %d: every page of the repository is stale", n, 2*len(pages))
			}
		})
	}
}

func TestTimelineErrorWrappedAndNotCached(t *testing.T) {
	for _, r := range timelineReads {
		t.Run(r.name, func(t *testing.T) {
			api := &fakeAPI{}
			r.serve(api, func(pageRequest) (page, error) { return page{}, core.ErrNotFound })
			s := New(api)
			q := pageRequest{repo: repo, number: 7}

			for range 2 {
				_, err := r.fetch(t.Context(), s, q)
				if !errors.Is(err, core.ErrNotFound) {
					t.Errorf("error = %v, want ErrNotFound", err)
				}
				if err != nil && !strings.Contains(err.Error(), "list "+r.name+" of pull eggzec/gh-tui#7") {
					t.Errorf("error %q lacks context", err)
				}
			}
			if n := api.count(r.method); n != 2 {
				t.Errorf("API called %d times, want 2: errors must not be cached", n)
			}
			if _, ok := r.cached(s, q); ok {
				t.Error("cached reported a page after a failed fetch")
			}
		})
	}
}

// The mutations change the state of a pull request, not its thread.
func TestTimelineKeptByMutation(t *testing.T) {
	for _, r := range timelineReads {
		t.Run(r.name, func(t *testing.T) {
			api := &fakeAPI{mutate: func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
				return openPull(1), nil
			}}
			r.serve(api, thread)
			s := seeded(t, api, func(*core.PullRequest) {})
			q := pageRequest{repo: repo, number: 1}
			before, err := r.fetch(t.Context(), s, q)
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}

			if err := s.Close(repo, 1).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			after, err := r.fetch(t.Context(), s, q)
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if !reflect.DeepEqual(after, before) || api.count(r.method) != 1 {
				t.Errorf("page = %+v after %d calls, want %+v from the cache", after, api.count(r.method), before)
			}
		})
	}
}

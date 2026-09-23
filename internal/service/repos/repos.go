// Package repos serves the viewer's repositories from a cache.
package repos

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// DefaultPageSize is the page size of a ListQuery that sets none.
const DefaultPageSize = 30

// API is the part of the GitHub client the service uses.
type API interface {
	ListRepos(ctx context.Context, first int, after string) (core.Page[core.Repo], error)
	GetRepo(ctx context.Context, ref core.RepoRef) (core.Repo, error)
}

// ListQuery selects a page of the viewer's repositories.
type ListQuery struct {
	// After is the Next cursor of the previous page, or empty for the first.
	After string
	// First is the page size. Zero means DefaultPageSize.
	First int
}

func (q ListQuery) normalize() ListQuery {
	if q.First <= 0 {
		q.First = DefaultPageSize
	}
	return q
}

// Service reads repositories through a cache. It is safe for concurrent use.
type Service struct {
	api   API
	lists *cache.Cache[core.Page[core.Repo]]
	repos *cache.Cache[core.Repo]
}

// New returns a Service that fetches from api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	copts := []cache.Option{cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)}
	return &Service{
		api:   api,
		lists: cache.New[core.Page[core.Repo]](copts...),
		repos: cache.New[core.Repo](copts...),
	}
}

// CachedList returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedList(q ListQuery) (core.Page[core.Repo], bool) {
	e, state := s.lists.Get(listKey(q.normalize()))
	return e.Value, state != cache.Miss
}

// List returns the page for q. A fresh cached page is returned without a
// request.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Repo], error) {
	q = q.normalize()
	e, err := s.lists.Fetch(ctx, listKey(q), func(ctx context.Context, _ cache.Entry[core.Page[core.Repo]], _ bool) (cache.Entry[core.Page[core.Repo]], error) {
		// GraphQL has no validators, so a stale page is fetched again in full.
		p, err := s.api.ListRepos(ctx, q.First, q.After)
		if err != nil {
			return cache.Entry[core.Page[core.Repo]]{}, err
		}
		tags := make([]string, len(p.Items))
		for i := range p.Items {
			tags[i] = repoTag(p.Items[i].Ref)
		}
		return cache.Entry[core.Page[core.Repo]]{Value: p, Tags: tags}, nil
	})
	if err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos: %w", err)
	}
	return e.Value, nil
}

// CachedGet returns the cached repository, fresh or stale, without I/O.
func (s *Service) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	e, state := s.repos.Get(repoKey(ref))
	return e.Value, state != cache.Miss
}

// Get returns one repository. A fresh cached repository is returned without
// a request.
func (s *Service) Get(ctx context.Context, ref core.RepoRef) (core.Repo, error) {
	e, err := s.repos.Fetch(ctx, repoKey(ref), func(ctx context.Context, _ cache.Entry[core.Repo], _ bool) (cache.Entry[core.Repo], error) {
		r, err := s.api.GetRepo(ctx, ref)
		if err != nil {
			return cache.Entry[core.Repo]{}, err
		}
		return cache.Entry[core.Repo]{Value: r, Tags: []string{repoTag(ref)}}, nil
	})
	if err != nil {
		return core.Repo{}, fmt.Errorf("get repo %s: %w", ref, err)
	}
	return e.Value, nil
}

func listKey(q ListQuery) string {
	return "list?first=" + strconv.Itoa(q.First) + "&after=" + q.After
}

// GitHub matches owners and names without regard to case, so keys and tags
// do too.

func repoKey(ref core.RepoRef) string {
	return "repo/" + strings.ToLower(ref.String())
}

// repoTag marks every entry, list page or repository, that holds ref.
func repoTag(ref core.RepoRef) string {
	return "repo:" + strings.ToLower(ref.String())
}

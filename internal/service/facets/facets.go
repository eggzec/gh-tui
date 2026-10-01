// Package facets serves what the lists of a repository can be filtered by:
// its labels, its open milestones and the people who work on it. Filters
// offer them as options, so each is read once and cached, and labels and
// milestones are revalidated with a free 304.
package facets

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// API is the part of the GitHub client the service uses.
type API interface {
	ListLabels(ctx context.Context, repo core.RepoRef, cond github.Conditional) ([]core.Label, github.Response, error)
	ListMilestones(ctx context.Context, repo core.RepoRef, cond github.Conditional) ([]core.Milestone, github.Response, error)
	RepoUsers(ctx context.Context, repo core.RepoRef, query string, first int) ([]core.User, error)
}

// Service reads the facets of repositories through caches. It is safe for
// concurrent use.
type Service struct {
	api        API
	labels     *cache.Cache[[]core.Label]
	milestones *cache.Cache[[]core.Milestone]
	people     *cache.Cache[[]core.User]
}

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl      time.Duration
	capacity int
}

// WithTTL sets how long what was read stays fresh. Without it, or with d
// at or below zero, it is the default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithCapacity sets how many lists of labels, of milestones and of people
// are each kept. Without it, or with n below one, it is the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// New returns a service that reads from api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default().Cache
	mem := []cache.Option{cache.WithTTL(cmp.Or(o.ttl, d.TTL.Filters)), cache.WithCapacity(cmp.Or(o.capacity, d.Memory.Entries))}
	return &Service{
		api:        api,
		labels:     cache.New[[]core.Label](mem...),
		milestones: cache.New[[]core.Milestone](mem...),
		people:     cache.New[[]core.User](mem...),
	}
}

// PeopleQuery selects the people of a repository that match Text, a part
// of a login or a name. An empty Text selects those who can be assigned.
type PeopleQuery struct {
	Repo core.RepoRef
	Text string
}

// peopleLimit is how many people a read returns: a picker's worth.
const peopleLimit = 20

func (q PeopleQuery) key() string {
	return "people:" + repoID(q.Repo) + "?" + strings.ToLower(strings.TrimSpace(q.Text))
}

func labelsKey(repo core.RepoRef) string     { return "labels:" + repoID(repo) }
func milestonesKey(repo core.RepoRef) string { return "milestones:" + repoID(repo) }

// repoID names a repository in keys. GitHub ignores case in names.
func repoID(r core.RepoRef) string { return strings.ToLower(r.String()) }

// CachedLabels returns the labels of repo if they are cached, fresh or
// stale, without a request.
func (s *Service) CachedLabels(repo core.RepoRef) ([]core.Label, bool) {
	return cached(s.labels, labelsKey(repo))
}

// Labels returns the labels of repo, up to 100. Cached ones are returned
// while they are fresh, and revalidated once they are stale.
func (s *Service) Labels(ctx context.Context, repo core.RepoRef) ([]core.Label, error) {
	v, err := revalidated(ctx, s.labels, labelsKey(repo), func(ctx context.Context, cond github.Conditional) ([]core.Label, github.Response, error) {
		return s.api.ListLabels(ctx, repo, cond)
	})
	if err != nil {
		return nil, fmt.Errorf("list labels of %s: %w", repo, err)
	}
	return v, nil
}

// CachedMilestones returns the open milestones of repo if they are cached,
// fresh or stale, without a request.
func (s *Service) CachedMilestones(repo core.RepoRef) ([]core.Milestone, bool) {
	return cached(s.milestones, milestonesKey(repo))
}

// Milestones returns the open milestones of repo, up to 100, the one due
// soonest first, as Labels does.
func (s *Service) Milestones(ctx context.Context, repo core.RepoRef) ([]core.Milestone, error) {
	v, err := revalidated(ctx, s.milestones, milestonesKey(repo), func(ctx context.Context, cond github.Conditional) ([]core.Milestone, github.Response, error) {
		return s.api.ListMilestones(ctx, repo, cond)
	})
	if err != nil {
		return nil, fmt.Errorf("list milestones of %s: %w", repo, err)
	}
	return v, nil
}

// CachedPeople returns the people of q if they are cached, fresh or stale,
// without a request.
func (s *Service) CachedPeople(q PeopleQuery) ([]core.User, bool) {
	return cached(s.people, q.key())
}

// People returns up to 20 people of q: those who can be assigned first,
// then, once q has text, other accounts whose login it starts. GraphQL has
// no validators, so a stale answer is read again in full.
func (s *Service) People(ctx context.Context, q PeopleQuery) ([]core.User, error) {
	e, err := s.people.Fetch(ctx, q.key(), func(ctx context.Context, _ cache.Entry[[]core.User], _ bool) (cache.Entry[[]core.User], error) {
		users, err := s.api.RepoUsers(ctx, q.Repo, q.Text, peopleLimit)
		return cache.Entry[[]core.User]{Value: users}, err
	})
	if err != nil {
		return nil, fmt.Errorf("list people of %s: %w", q.Repo, err)
	}
	return e.Value, nil
}

func cached[V any](c *cache.Cache[V], key string) (V, bool) {
	e, st := c.Get(key)
	return e.Value, st != cache.Miss
}

// revalidated returns the value under key in c, read with load when it is
// missing, and revalidated with its ETag when it is stale.
func revalidated[V any](ctx context.Context, c *cache.Cache[V], key string, load func(ctx context.Context, cond github.Conditional) (V, github.Response, error)) (V, error) {
	e, err := c.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		v, res, err := load(ctx, cond)
		switch {
		case err != nil:
			return cache.Entry[V]{}, err
		case res.NotModified:
			return cache.Entry[V]{}, cache.ErrNotModified
		}
		return cache.Entry[V]{Value: v, ETag: res.ETag, LastModified: res.LastModified}, nil
	})
	return e.Value, err
}

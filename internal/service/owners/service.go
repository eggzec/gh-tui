// Package owners serves what the page of a user or an organization shows
// from a cache: the account's header with its pins, its repositories, its
// stars, its people and teams, its sponsors, its profile README and, for
// a user, the contribution calendar.
package owners

import (
	"cmp"
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// API is the part of the GitHub client the service uses.
type API interface {
	OwnerHeader(ctx context.Context, login string) (core.Owner, error)
	UserRepos(ctx context.Context, login string, order core.RepoOrder, first int, after string) (core.Page[core.Repo], error)
	OrgRepos(ctx context.Context, login string, first int, after string) (core.Page[core.Repo], error)
	UserContributions(ctx context.Context, login string) (core.Contributions, error)
	UserStars(ctx context.Context, login string, first int, after string) (core.Page[core.Repo], error)
	UserFollowers(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error)
	UserFollowing(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error)
	UserOrgs(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error)
	OrgMembers(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error)
	OrgTeams(ctx context.Context, login string, first int, after string) (core.Page[core.Team], error)
	OwnerSponsors(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error)
	OwnerSponsoring(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error)
	ProfileReadme(ctx context.Context, login string, kind core.OwnerKind, member bool, cond github.Conditional) (core.Readme, github.Response, error)
	OrgFollowers(ctx context.Context, login string) (int, error)
}

// Service reads the pages of users and organizations through a cache. It
// is safe for concurrent use.
type Service struct {
	api           API
	header        reads[core.Owner]
	repos         reads[core.Page[core.Repo]]
	contributions reads[core.Contributions]
	people        reads[core.Page[core.Person]]
	teams         reads[core.Page[core.Team]]
	readme        reads[Readme]
	followers     reads[FollowerCount]
	// missing holds the logins that GitHub said no account has, for a
	// moment, so that reading one again doesn't ask at once.
	missing *cache.Cache[error]
	// membersOnly holds the organizations whose teams GitHub showed the
	// viewer only to members, for TTLs.People, so that the tab of teams
	// read again doesn't ask at once.
	membersOnly *cache.Cache[error]
	// noSponsors is set once GitHub said it has no GitHub Sponsors, as an
	// Enterprise Server says, which holds for the whole session.
	noSponsors atomic.Bool
	// ttls and sizes are what the reads of each kind use, those of the
	// config where an option set none.
	ttls  TTLs
	sizes Sizes
}

// The kinds of entries the service keeps in its store, and the version of
// their values. Bump schema when a core type they hold changes shape.
const (
	kindHeader        = "owner"
	kindRepos         = "ownerpagerepos"
	kindContributions = "ownercontrib"
	kindPeople        = "ownerpeople"
	kindTeams         = "ownerteams"
	kindReadme        = "ownerreadme"
	kindFollowers     = "ownerfollowers"
	schema            = 1
)

// missingFor is how long a login that no account has is remembered: long
// enough that a page read again at once doesn't ask, and short enough
// that an account created since soon shows.
const missingFor = time.Minute

// New returns a Service that fetches from api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	ttl := d.Cache.TTL
	t := o.ttls
	t.Header = positive(t.Header, ttl.Profile)
	t.Repos = positive(t.Repos, ttl.DashboardRepos)
	t.Contributions = positive(t.Contributions, ttl.Contributions)
	t.People = positive(t.People, ttl.People)
	t.Readme = positive(t.Readme, ttl.Readme)
	sz := o.sizes
	sz.Repos = cmp.Or(sz.Repos, d.PageSize.Repos)
	sz.People = cmp.Or(sz.People, d.PageSize.People)
	o.capacity = cmp.Or(o.capacity, d.Cache.Memory.Entries)
	return &Service{
		api: api,
		header: newReads(o, kindHeader, t.Header, func(h *core.Owner) (*bool, *bool, *bool) {
			return &h.Stale, &h.Offline, &h.Limited
		}),
		repos: newReads(o, kindRepos, t.Repos, func(p *core.Page[core.Repo]) (*bool, *bool, *bool) {
			return &p.Stale, &p.Offline, &p.Limited
		}),
		contributions: newReads(o, kindContributions, t.Contributions, func(c *core.Contributions) (*bool, *bool, *bool) {
			return &c.Stale, &c.Offline, &c.Limited
		}),
		people: newReads(o, kindPeople, t.People, func(p *core.Page[core.Person]) (*bool, *bool, *bool) {
			return &p.Stale, &p.Offline, &p.Limited
		}),
		teams: newReads(o, kindTeams, t.People, func(p *core.Page[core.Team]) (*bool, *bool, *bool) {
			return &p.Stale, &p.Offline, &p.Limited
		}),
		readme: newReads(o, kindReadme, t.Readme, func(r *Readme) (*bool, *bool, *bool) {
			return &r.Stale, &r.Offline, &r.Limited
		}),
		followers: newReads(o, kindFollowers, t.Header, func(f *FollowerCount) (*bool, *bool, *bool) {
			return &f.Stale, &f.Offline, &f.Limited
		}),
		missing:     cache.New[error](cache.WithTTL(missingFor), cache.WithCapacity(o.capacity)),
		membersOnly: cache.New[error](cache.WithTTL(t.People), cache.WithCapacity(o.capacity)),
		ttls:        t,
		sizes:       sz,
	}
}

// positive returns d, or def if d isn't positive.
func positive(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// Invalidate marks everything the service cached stale, and forgets the
// logins it found no account for and the organizations whose teams it
// was refused. What is stale is still served by the
// Cached reads, and the next read of each entry goes to GitHub, so a
// refresh reaches the server even while the entries are fresh.
func (s *Service) Invalidate() {
	s.header.mem.InvalidateTag(allTag)
	s.repos.mem.InvalidateTag(allTag)
	s.contributions.mem.InvalidateTag(allTag)
	s.people.mem.InvalidateTag(allTag)
	s.teams.mem.InvalidateTag(allTag)
	s.readme.mem.InvalidateTag(allTag)
	s.followers.mem.InvalidateTag(allTag)
	s.missing.InvalidateTag(allTag)
	s.membersOnly.InvalidateTag(allTag)
}

// allTag marks every entry, so that Invalidate finds them all.
const allTag = "all"

// loginKey is login as the keys hold it: GitHub matches logins without
// regard to case, so keys do too.
func loginKey(login string) string {
	return strings.ToLower(login)
}

// read reads the value of login through r under key, unless GitHub said
// a moment ago that no account has that login, or that this kind of read,
// scope, finds nothing for it. The header, whose scope is empty, asks
// about the account whatever its kind, so its answer covers every read of
// the login; another read can miss for an account of the other kind, as
// the calendar of an organization does, so its answer covers its scope
// alone. Either is remembered for missingFor.
func read[V any](ctx context.Context, s *Service, r *reads[V], login, scope, key string, again bool, load cache.FetchFunc[V]) (V, error) {
	login = loginKey(login)
	for _, k := range missingKeys(login, scope) {
		if e, state := s.missing.Get(k); state == cache.Fresh {
			var zero V
			return zero, e.Value
		}
	}
	v, err := r.get(ctx, key, again, load)
	if core.KindOf(err) == core.NotFound {
		keys := missingKeys(login, scope)
		s.missing.Set(keys[len(keys)-1], cache.Entry[error]{Value: err, Tags: []string{allTag}})
	}
	return v, err
}

// missingKeys are the keys of s.missing that a read of scope checks: the
// login's, then its scope's, if any.
func missingKeys(login, scope string) []string {
	if scope == "" {
		return []string{login}
	}
	return []string{login, scope + ":" + login}
}

// reads is one kind of read: its cache in memory, its shelf in the store,
// and how to mark a value served stale, offline or limited.
type reads[V any] struct {
	mem   *cache.Cache[V]
	kept  *cache.Shelf[V]
	flags func(*V) (stale, offline, limited *bool)
}

func newReads[V any](o options, kind string, ttl time.Duration, flags func(*V) (stale, offline, limited *bool)) reads[V] {
	return reads[V]{
		mem:   cache.New[V](cache.WithTTL(ttl), cache.WithCapacity(o.capacity)),
		kept:  cache.NewShelf[V](o.store, kind, schema),
		flags: flags,
	}
}

// cached returns the value under key in memory, fresh or stale, without
// I/O.
func (r *reads[V]) cached(key string) (V, bool) {
	e, state := r.mem.Get(key)
	return e.Value, state != cache.Miss
}

// fresh reports whether the value under key is in memory and fresh, so
// that reading it costs no request, without I/O.
func (r *reads[V]) fresh(key string) bool {
	_, state := r.mem.Get(key)
	return state == cache.Fresh
}

// get returns the value under key. A fresh value in memory is returned
// without a request, and so is one kept by an earlier session within the
// TTL. An older kept one is returned at once, marked stale, until a read
// with again set fetches it. Otherwise get loads the value, stores it and
// keeps it, falling back on the stale value as fallback.Fetch does.
func (r *reads[V]) get(ctx context.Context, key string, again bool, load cache.FetchFunc[V]) (V, error) {
	if e, ok := r.kept.Warm(r.mem, key, again); ok {
		v := e.Value
		stale, _, _ := r.flags(&v)
		*stale = true
		return v, nil
	}
	e, err := fallback.Fetch(ctx, r.mem, r.kept, key, r.marks, fallback.Keep(r.kept, key, load))
	return e.Value, err
}

// whole adapts fetch, a read without validators such as a GraphQL query,
// to a cache.FetchFunc: a stale value is fetched again in full.
func whole[V any](fetch func(context.Context) (V, error)) cache.FetchFunc[V] {
	return func(ctx context.Context, _ cache.Entry[V], _ bool) (cache.Entry[V], error) {
		v, err := fetch(ctx)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		return cache.Entry[V]{Value: v, Tags: []string{allTag}}, nil
	}
}

// marks are the fallback.Marks of the values.
func (r *reads[V]) marks(v *V) (offline, limited *bool) {
	_, offline, limited = r.flags(v)
	return offline, limited
}

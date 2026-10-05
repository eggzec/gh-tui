package owners

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// PeopleList is a list of accounts that the page of an account shows.
type PeopleList int

// The lists of people. Followers, Following and Orgs are a user's,
// Members an organization's, and Sponsors and Sponsoring either's.
const (
	Followers PeopleList = iota
	Following
	Orgs
	Members
	Sponsors
	Sponsoring
)

var peopleListNames = [...]string{"followers", "following", "orgs", "members", "sponsors", "sponsoring"}

// String names the list, as its keys do.
func (l PeopleList) String() string {
	if l < 0 || int(l) >= len(peopleListNames) {
		return "people" + strconv.Itoa(int(l))
	}
	return peopleListNames[l]
}

// sponsors reports whether the list is of GitHub Sponsors, which only
// github.com has.
func (l PeopleList) sponsors() bool {
	return l == Sponsors || l == Sponsoring
}

// errNoSponsors is why a list of sponsors fails once GitHub said it has
// none.
var errNoSponsors = fmt.Errorf("GitHub Sponsors is on github.com only: %w", core.ErrUnsupported)

// PeopleQuery selects a page of a list of people of an account.
type PeopleQuery struct {
	// Login is the user's or the organization's.
	Login string
	List  PeopleList
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many accounts a page holds. Zero means the service's
	// size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q PeopleQuery) normalize(size int) PeopleQuery {
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

// pageSize is size, or def if size isn't positive, at most maxPageSize.
func pageSize(size, def int) int {
	if size <= 0 {
		size = def
	}
	return min(size, maxPageSize)
}

func (q PeopleQuery) key() string {
	v := url.Values{"list": {q.List.String()}, "first": {strconv.Itoa(q.PageSize)}, "cursor": {q.Cursor}}
	return "ownerpeople:" + loginKey(q.Login) + "?" + v.Encode()
}

// CachedPeople returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedPeople(q PeopleQuery) (core.Page[core.Person], bool) {
	return s.people.cached(q.normalize(s.sizes.People).key())
}

// FreshPeople reports whether the page for q is cached and fresh, so that
// reading it costs no request. It does no I/O.
func (s *Service) FreshPeople(q PeopleQuery) bool {
	return s.people.fresh(q.normalize(s.sizes.People).key())
}

// People returns the page for q. It is fresh for TTLs.People, and is
// served stale, offline or limited as in Header. A login that no account
// has, or whose account has no such list, as an organization has no
// followers here, fails with an error matching core.ErrNotFound, and so
// does that list alone again for a moment without a request; the
// account's other lists still ask.
//
// The lists of sponsors fail with an error matching core.ErrUnsupported
// where GitHub has no GitHub Sponsors, as on an Enterprise Server, and
// once it said so they fail so for the rest of the session without a
// request.
func (s *Service) People(ctx context.Context, q PeopleQuery) (core.Page[core.Person], error) {
	q = q.normalize(s.sizes.People)
	if q.List.sponsors() && s.noSponsors.Load() {
		return core.Page[core.Person]{}, fmt.Errorf("%s of %s: %w", q.List, q.Login, errNoSponsors)
	}
	// The client already names the request in its error.
	p, err := read(ctx, s, &s.people, q.Login, q.List.String(), q.key(), q.Again, whole(func(ctx context.Context) (core.Page[core.Person], error) {
		return s.listPeople(ctx, q)
	}))
	if q.List.sponsors() && errors.Is(err, core.ErrUnsupported) {
		s.noSponsors.Store(true)
	}
	return p, err
}

func (s *Service) listPeople(ctx context.Context, q PeopleQuery) (core.Page[core.Person], error) {
	switch q.List {
	case Followers:
		return s.api.UserFollowers(ctx, q.Login, q.PageSize, q.Cursor)
	case Following:
		return s.api.UserFollowing(ctx, q.Login, q.PageSize, q.Cursor)
	case Orgs:
		return s.api.UserOrgs(ctx, q.Login, q.PageSize, q.Cursor)
	case Members:
		return s.api.OrgMembers(ctx, q.Login, q.PageSize, q.Cursor)
	case Sponsors:
		return s.api.OwnerSponsors(ctx, q.Login, q.PageSize, q.Cursor)
	case Sponsoring:
		return s.api.OwnerSponsoring(ctx, q.Login, q.PageSize, q.Cursor)
	default:
		return core.Page[core.Person]{}, fmt.Errorf("list %s of %s: no such list", q.List, q.Login)
	}
}

// TeamsQuery selects a page of the teams of an organization.
type TeamsQuery struct {
	Login string
	// Cursor, PageSize and Again work as in PeopleQuery.
	Cursor   string
	PageSize int
	Again    bool
}

func (q TeamsQuery) normalize(size int) TeamsQuery {
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

func (q TeamsQuery) key() string {
	v := url.Values{"first": {strconv.Itoa(q.PageSize)}, "cursor": {q.Cursor}}
	return "ownerteams:" + loginKey(q.Login) + "?" + v.Encode()
}

// CachedTeams returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedTeams(q TeamsQuery) (core.Page[core.Team], bool) {
	return s.teams.cached(q.normalize(s.sizes.People).key())
}

// FreshTeams reports whether the page for q is cached and fresh, so that
// reading it costs no request. It does no I/O.
func (s *Service) FreshTeams(q TeamsQuery) bool {
	return s.teams.fresh(q.normalize(s.sizes.People).key())
}

// Teams returns the page for q, by name. It is fresh for TTLs.People, and
// is served stale, offline or limited, or fails for an organization that
// isn't there, as in People.
//
// Only an organization's members see its teams: for anyone else Teams
// fails with an error matching core.ErrForbidden, which is how the page
// learns that the teams are for members only rather than that something
// went wrong. Nothing of them is then kept, and the answer holds for
// TTLs.People without a request, or until Invalidate.
func (s *Service) Teams(ctx context.Context, q TeamsQuery) (core.Page[core.Team], error) {
	q = q.normalize(s.sizes.People)
	login := loginKey(q.Login)
	if e, state := s.membersOnly.Get(login); state == cache.Fresh {
		return core.Page[core.Team]{}, e.Value
	}
	// The client already names the request in its error.
	p, err := read(ctx, s, &s.teams, q.Login, "teams", q.key(), q.Again, whole(func(ctx context.Context) (core.Page[core.Team], error) {
		return s.api.OrgTeams(ctx, q.Login, q.PageSize, q.Cursor)
	}))
	if errors.Is(err, core.ErrForbidden) {
		s.membersOnly.Set(login, cache.Entry[error]{Value: err, Tags: tags(login)})
	}
	return p, err
}

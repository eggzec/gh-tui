package owners

import (
	"context"
	"net/url"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// Readme is the profile README of an account, as it was served.
type Readme struct {
	core.Readme
	// Stale, Offline and Limited say how the README was served, as those
	// of core.Owner do.
	Stale, Offline, Limited bool
}

// ReadmeQuery selects the profile README of an account.
type ReadmeQuery struct {
	// Login is the user's or the organization's, and Kind which of the
	// two it is, as its header says.
	Login string
	Kind  core.OwnerKind
	// Member reports that the viewer is a member of the organization, as
	// its header says, so that the README only members see comes first.
	// A user's README ignores it.
	Member bool
	// Again reads past a kept README: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q ReadmeQuery) normalize() ReadmeQuery {
	if q.Kind != core.OwnerOrg {
		q.Member = false
	}
	return q
}

const readmePrefix = "ownerreadme:"

// key names the README of the query. Its kind and the viewer's membership
// are in it, as a check in the background needs them. They also keep a
// README read as a member apart from one read as anyone else: GitHub
// sends the same ETag for the same README in either repository it may
// come from, so validators from a read with the other membership could
// confirm the wrong one, and a change of membership reads the README
// again without them.
func (q ReadmeQuery) key() string {
	q = q.normalize()
	v := url.Values{"kind": {"user"}}
	if q.Kind == core.OwnerOrg {
		v.Set("kind", "org")
		if q.Member {
			v.Set("member", "1")
		}
	}
	return readmePrefix + loginKey(q.Login) + "?" + v.Encode()
}

// parseReadmeKey returns the query that key names.
func parseReadmeKey(key string) (ReadmeQuery, bool) {
	rest, ok := strings.CutPrefix(key, readmePrefix)
	if !ok {
		return ReadmeQuery{}, false
	}
	login, raw, ok := strings.Cut(rest, "?")
	if !ok || login == "" {
		return ReadmeQuery{}, false
	}
	v, err := url.ParseQuery(raw)
	if err != nil {
		return ReadmeQuery{}, false
	}
	q := ReadmeQuery{Login: login, Member: v.Get("member") == "1"}
	switch v.Get("kind") {
	case "user":
	case "org":
		q.Kind = core.OwnerOrg
	default:
		return ReadmeQuery{}, false
	}
	return q, q.key() == key
}

// CachedReadme returns the cached README for q, fresh or stale, without
// I/O.
func (s *Service) CachedReadme(q ReadmeQuery) (Readme, bool) {
	return s.readme.cached(q.key())
}

// FreshReadme reports whether the README for q is cached and fresh, so
// that reading it costs no request. It does no I/O.
func (s *Service) FreshReadme(q ReadmeQuery) bool {
	return s.readme.fresh(q.key())
}

// Readme returns the profile README of the account q.Login: a user's from
// the repository named after them, and an organization's from its .github
// repository, or, for a member, from its .github-private one first. An
// account without one is not an error: the README's Source is then the
// zero RepoRef.
//
// It is fresh for TTLs.Readme, then read again with its validators, which
// costs no rate limit when it didn't change, and is kept with them, so
// that the revalidator checks it in the background (Kept). It is served
// stale, offline or limited as in People.
func (s *Service) Readme(ctx context.Context, q ReadmeQuery) (Readme, error) {
	q = q.normalize()
	// The client already names the request in its error.
	return read(ctx, s, &s.readme, q.Login, "readme", q.key(), q.Again, s.loadReadme(q))
}

// loadReadme reads the README for q, with the validators of the one read
// last, if any. A 304 leaves the README read last in place, its Source
// with it, since GitHub doesn't send it again. But the same README in two
// repositories has the same ETag, so a 304 from a repository other than
// the README's own, such as .github once .github-private is refused,
// would keep the wrong label: the README is then read again without
// validators. Repository names are compared without regard to case, as
// a user's README repository is named after the login, which GitHub
// ignores the case of, and the revalidator asks with it lowercased.
func (s *Service) loadReadme(q ReadmeQuery) cache.FetchFunc[Readme] {
	return func(ctx context.Context, prev cache.Entry[Readme], ok bool) (cache.Entry[Readme], error) {
		return recheck.Load(func(ctx context.Context, cond github.Conditional) (Readme, github.Response, error) {
			r, res, err := s.api.ProfileReadme(ctx, q.Login, q.Kind, q.Member, cond)
			if err == nil && res.NotModified && ok && !strings.EqualFold(r.Source.Name, prev.Value.Source.Name) {
				r, res, err = s.api.ProfileReadme(ctx, q.Login, q.Kind, q.Member, github.Conditional{})
			}
			return Readme{Readme: r}, res, err
		}, func(Readme) []string { return tags(q.Login) })(ctx, prev, ok)
	}
}

// SyncKey names changes to what the page of the account login shows in
// sync events, such as a README that the revalidator found changed.
func SyncKey(login string) string {
	return "owner:" + loginKey(login)
}

func (s *Service) readmeTarget(key string) (recheck.Target, bool) {
	q, ok := parseReadmeKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Check: func(ctx context.Context) revalidate.Result {
		_, res := recheck.Check(ctx, s.readme.mem, s.readme.kept, key, SyncKey(q.Login), s.loadReadme(q))
		return res
	}}, true
}

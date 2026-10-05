package owners

import (
	"context"
)

// FollowerCount is how many follow an organization.
type FollowerCount struct {
	Count int
	// Stale, Offline and Limited say how the count was served, as those of
	// core.Owner do.
	Stale, Offline, Limited bool
}

func followersKey(login string) string {
	return "ownerfollowers:" + loginKey(login)
}

// CachedOrgFollowers returns the cached follower count of the
// organization login, fresh or stale, without I/O.
func (s *Service) CachedOrgFollowers(login string) (FollowerCount, bool) {
	return s.followers.cached(followersKey(login))
}

// FreshOrgFollowers reports whether the follower count of the
// organization login is cached and fresh, so that reading it costs no
// request. It does no I/O.
func (s *Service) FreshOrgFollowers(login string) bool {
	return s.followers.fresh(followersKey(login))
}

// OrgFollowersQuery selects the follower count of an organization.
type OrgFollowersQuery struct {
	Login string
	// Again reads past a kept count: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// OrgFollowers returns how many follow the organization q.Login, which
// the header can't count. It is part of the profile, so it is fresh for
// TTLs.Header, and is served stale, offline or limited, or fails for an
// account that isn't there, as in People.
func (s *Service) OrgFollowers(ctx context.Context, q OrgFollowersQuery) (FollowerCount, error) {
	// The client already names the request in its error.
	return read(ctx, s, &s.followers, q.Login, "orgfollowers", followersKey(q.Login), q.Again, whole(func(ctx context.Context) (FollowerCount, error) {
		n, err := s.api.OrgFollowers(ctx, q.Login)
		return FollowerCount{Count: n}, err
	}))
}

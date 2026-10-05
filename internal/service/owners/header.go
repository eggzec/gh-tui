package owners

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func headerKey(login string) string {
	return "owner:" + loginKey(login)
}

// CachedHeader returns the cached header of login, fresh or stale,
// without I/O.
func (s *Service) CachedHeader(login string) (core.Owner, bool) {
	return s.header.cached(headerKey(login))
}

// FreshHeader reports whether the header of login is cached and fresh, so
// that reading it costs no request. It does no I/O.
func (s *Service) FreshHeader(login string) bool {
	return s.header.fresh(headerKey(login))
}

// HeaderQuery selects the header of an account.
type HeaderQuery struct {
	// Login is the user's or the organization's.
	Login string
	// Again reads past a kept header: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// Header returns the profile, the counts, the pinned repositories and the
// viewer's relation of the user or organization q.Login. It is fresh for
// TTLs.Header. A header an earlier session kept comes back at once with
// Stale set once that has passed, until a read with q.Again set fetches
// it; if GitHub can't be reached, the last one is served with Offline
// set, and with Limited set if it rate limited the read. A login that no
// account has fails with an error matching core.ErrNotFound, and fails so
// again for a moment without a request.
//
// When GitHub answers only part of the header, as when it hides a pinned
// repository from the token, the part it answered is the header.
func (s *Service) Header(ctx context.Context, q HeaderQuery) (core.Owner, error) {
	// The client already names the request in its error.
	return read(ctx, s, &s.header, q.Login, "", headerKey(q.Login), q.Again, func(ctx context.Context) (core.Owner, error) {
		o, err := s.api.OwnerHeader(ctx, q.Login)
		if err != nil && partial(o, err) {
			slog.DebugContext(ctx, "owner header partly refused", "login", q.Login, "err", err)
			return o, nil
		}
		return o, err
	})
}

// partial reports whether err is GitHub refusing only some fields of the
// header o it answered, such as a pinned repository hidden from the token:
// every error points below the account, and says nothing the app knows or
// that the field is hidden or gone. A rate limit, an outage, a scope the
// token lacks, or an error about the account itself fails the whole read.
func partial(o core.Owner, err error) bool {
	var gerr *github.GraphQLError
	if o.ID == "" && o.Profile.Login == "" || !errors.As(err, &gerr) || len(gerr.Errors) == 0 {
		return false
	}
	switch core.KindOf(err) {
	case core.Internal, core.Forbidden, core.NotFound:
	default:
		return false
	}
	for _, item := range gerr.Errors {
		// The path of an error about the account itself ends there, as
		// ["repositoryOwner"], and one about the query has none.
		if len(item.Path) < 2 {
			return false
		}
	}
	return true
}

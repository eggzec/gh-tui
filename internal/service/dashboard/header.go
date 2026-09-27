package dashboard

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
)

const headerKey = "dashheader:viewer"

// CachedHeader returns the cached header, fresh or stale, without I/O.
func (s *Service) CachedHeader() (core.Header, bool) {
	return s.header.cached(headerKey)
}

// FreshHeader reports whether the header is cached and fresh, so that
// reading it costs no request. It does no I/O.
func (s *Service) FreshHeader() bool {
	return s.header.fresh(headerKey)
}

// HeaderQuery selects how the header is read.
type HeaderQuery struct {
	// Again reads past a kept header: set it on the read that follows one
	// that came back Stale.
	Again bool
}

// Header returns the viewer's profile, pinned repositories and
// organizations. It is fresh for HeaderTTL. A header an earlier session
// kept comes back at once with Stale set once that has passed, until a
// read with q.Again set fetches it; if GitHub can't be reached, the last
// one is served with Offline set.
func (s *Service) Header(ctx context.Context, q HeaderQuery) (core.Header, error) {
	// The client already names the request in its error.
	return s.header.get(ctx, headerKey, q.Again, s.api.ViewerHeader)
}

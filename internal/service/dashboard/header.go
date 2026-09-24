package dashboard

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

const headerKey = "dashheader:viewer"

// CachedHeader returns the cached header, fresh or stale, without I/O.
func (s *Service) CachedHeader() (core.Header, bool) {
	return s.header.cached(headerKey)
}

// Header returns the viewer's profile, pinned repositories and
// organizations. It is fresh for HeaderTTL. A header an earlier session
// kept comes back at once with Stale set once that has passed, and reading
// it again fetches it; if GitHub can't be reached, the last one is served
// with Offline set.
func (s *Service) Header(ctx context.Context) (core.Header, error) {
	h, err := s.header.get(ctx, headerKey, s.api.ViewerHeader)
	if err != nil {
		return core.Header{}, fmt.Errorf("dashboard header: %w", err)
	}
	return h, nil
}

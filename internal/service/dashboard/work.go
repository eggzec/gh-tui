package dashboard

import (
	"context"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// maxPageSize is the largest page GitHub returns.
const maxPageSize = 100

// WorkQuery selects the work waiting on the viewer.
type WorkQuery struct {
	// PageSize is how many of the most recently updated items each list
	// holds; the counts cover them all. Zero means the service's work
	// size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
	// Again reads past a kept value: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// normalize returns q with its defaults set: size is the service's work
// size.
func (q WorkQuery) normalize(size int) WorkQuery {
	if q.PageSize <= 0 {
		q.PageSize = size
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	return q
}

func (q WorkQuery) key() string {
	return "dashwork?first=" + strconv.Itoa(q.PageSize)
}

// CachedWork returns the cached work for q, fresh or stale, without I/O.
func (s *Service) CachedWork(q WorkQuery) (core.Work, bool) {
	return s.work.cached(q.normalize(s.workSize).key())
}

// FreshWork reports whether the work for q is cached and fresh, so that
// reading it costs no request. It does no I/O.
func (s *Service) FreshWork(q WorkQuery) bool {
	return s.work.fresh(q.normalize(s.workSize).key())
}

// Work returns the open pull requests that ask for the viewer's review,
// those they opened, and the issues assigned to them. It is fresh for the
// TTL of the service, and is served stale or offline as in Header.
func (s *Service) Work(ctx context.Context, q WorkQuery) (core.Work, error) {
	q = q.normalize(s.workSize)
	// The client already names the request in its error.
	return s.work.get(ctx, q.key(), q.Again, func(ctx context.Context) (core.Work, error) {
		return s.api.ViewerWork(ctx, q.PageSize)
	})
}

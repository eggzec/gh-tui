package pulls

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/seen"
)

// A list page shows when each pull request last changed. While that stands,
// what is cached of a pull request is current, so it is served past its TTL
// without asking GitHub, and a newer update time marks it stale at once.
//
// GitHub doesn't count check runs as updates, so the checks are compared as
// well, and a detail whose checks are still running is only as good as its
// TTL: the count of runs may move while the rollup state stands.

// mark is what a list page last showed of a pull request.
type mark struct {
	updated time.Time
	checks  core.ChecksState
}

// vouch records what a list page of repo showed of each of prs, and marks
// stale what is cached of those that changed since.
func (s *Service) vouch(repo core.RepoRef, prs []core.PullRequest) {
	for i := range prs {
		pr := &prs[i]
		key := detailKey(repo, pr.Number)
		prev, had := s.seen.Set(key, mark{updated: pr.UpdatedAt, checks: pr.Checks})
		e, st := s.details.Get(key)
		cached := st != cache.Miss
		older := cached && e.Value.UpdatedAt.Before(pr.UpdatedAt)
		if older || cached && e.Value.Checks != pr.Checks {
			s.details.Invalidate(key)
		}
		if older || had && prev.updated.Before(pr.UpdatedAt) {
			// Its comments and reviews may have changed too.
			s.comments.InvalidateTag(key)
			s.reviews.InvalidateTag(key)
		}
	}
}

// currentDetail returns the cached detail under key if it is current.
func (s *Service) currentDetail(key string) (core.PullRequestDetail, bool) {
	m, ok := s.seen.Get(key)
	if !ok || m.checks == core.ChecksPending {
		return core.PullRequestDetail{}, false
	}
	e, st := s.details.Get(key)
	d := e.Value
	// GitHub works out whether an open pull request merges after a push,
	// and says "unknown" until it has: that is no update of the pull
	// request, so such a detail is only as good as its TTL.
	working := d.State == core.StateOpen && d.Merge.Mergeable == core.MergeableUnknown
	return d, st != cache.Miss && !working && seen.Current(d.UpdatedAt, m.updated) && d.Checks == m.checks
}

// currentComments returns the cached page for q if it is current.
func (s *Service) currentComments(q CommentsQuery) (core.Page[core.Comment], bool) {
	m, ok := s.seen.Get(detailKey(q.Repo, q.Number))
	if !ok {
		return core.Page[core.Comment]{}, false
	}
	e, st := s.comments.Get(q.key(s.pageSize))
	return e.Value.Value, st != cache.Miss && seen.Current(e.Value.Version, m.updated)
}

// CurrentGet reports whether Get returns pull request number of repo
// without a request, since it is cached and fresh or current. It does no
// I/O.
func (s *Service) CurrentGet(repo core.RepoRef, number int) bool {
	key := detailKey(repo, number)
	_, ok := s.currentDetail(key)
	return ok || fresh(s.details, key)
}

// CurrentComments reports whether Comments returns the page of q without a
// request, since it is cached and fresh or current. It does no I/O.
func (s *Service) CurrentComments(q CommentsQuery) bool {
	_, ok := s.currentComments(q)
	return ok || fresh(s.comments, q.key(s.pageSize))
}

// CurrentFiles reports whether Files returns the page of q without a
// request, since it is cached and fresh. It does no I/O.
func (s *Service) CurrentFiles(q FilesQuery) bool {
	return q.Head != "" && fresh(s.files, q.key())
}

func fresh[V any](c *cache.Cache[V], key string) bool {
	_, st := c.Get(key)
	return st == cache.Fresh
}

// Changed tells the service that pull request number of repo last changed
// at updated, such as a notification of it says. What is cached of it from
// before then is marked stale, and a list page that showed it older no
// longer vouches for it, so that the next reads ask GitHub. It does no I/O.
func (s *Service) Changed(repo core.RepoRef, number int, updated time.Time) {
	if updated.IsZero() {
		return
	}
	key := detailKey(repo, number)
	if m, ok := s.seen.Get(key); ok && m.updated.Before(updated) {
		s.seen.Delete(key)
	}
	// A detail read before the change that already shows it is current.
	if e, st := s.details.Get(key); st != cache.Miss && e.FetchedAt.Before(updated) && e.Value.UpdatedAt.Before(updated) {
		s.details.Invalidate(key)
	}
	s.comments.InvalidateBefore(key, updated)
	s.reviews.InvalidateBefore(key, updated)
}

package issues

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/seen"
)

// A list page shows when each issue last changed. While that stands, what
// is cached of an issue is current, so it is served past its TTL without
// even a conditional request, and a newer update time marks it stale at
// once. A new comment counts as a change.

// vouch records when each of its a list page of repo showed them last
// changed, and marks stale what is cached of those that changed since.
func (s *Service) vouch(repo core.RepoRef, its []core.Issue) {
	for i := range its {
		it := &its[i]
		key := issueKey(repo, it.Number)
		prev, had := s.seen.Set(key, it.UpdatedAt)
		e, st := s.issues.Get(key)
		older := st != cache.Miss && e.Value.UpdatedAt.Before(it.UpdatedAt)
		if older {
			s.issues.Invalidate(key)
		}
		if older || had && prev.Before(it.UpdatedAt) {
			s.comments.InvalidateTag(key)
		}
	}
}

// currentIssue returns the cached issue under key if it is current.
func (s *Service) currentIssue(key string) (core.Issue, bool) {
	version, ok := s.seen.Get(key)
	if !ok {
		return core.Issue{}, false
	}
	e, st := s.issues.Get(key)
	return e.Value, st != cache.Miss && seen.Current(e.Value.UpdatedAt, version)
}

// currentComments returns the cached page for q, normalized, if it is
// current.
func (s *Service) currentComments(q CommentsQuery) (core.Page[core.Comment], bool) {
	version, ok := s.seen.Get(issueKey(q.Repo, q.Number))
	if !ok {
		return core.Page[core.Comment]{}, false
	}
	e, st := s.comments.Get(commentsKey(q))
	return e.Value.Value, st != cache.Miss && seen.Current(e.Value.Version, version)
}

// Current reports whether the issue of q and the page of comments q selects
// can both be read without a request, since they are cached and fresh or
// current.
func (s *Service) Current(q CommentsQuery) bool {
	q = q.normalize()
	key := issueKey(q.Repo, q.Number)
	if _, ok := s.currentIssue(key); !ok && !fresh(s.issues, key) {
		return false
	}
	if _, ok := s.currentComments(q); !ok && !fresh(s.comments, commentsKey(q)) {
		return false
	}
	return true
}

func fresh[V any](c *cache.Cache[V], key string) bool {
	_, st := c.Get(key)
	return st == cache.Fresh
}

// Changed tells the service that issue number of repo last changed at
// updated, such as a notification of it says. What is cached of it from
// before then is marked stale, and a list page that showed it older no
// longer vouches for it, so that the next reads ask GitHub. It does no I/O.
func (s *Service) Changed(repo core.RepoRef, number int, updated time.Time) {
	if updated.IsZero() {
		return
	}
	key := issueKey(repo, number)
	if v, ok := s.seen.Get(key); ok && v.Before(updated) {
		s.seen.Delete(key)
	}
	// An issue read before the change that already shows it is current.
	if e, st := s.issues.Get(key); st != cache.Miss && e.FetchedAt.Before(updated) && e.Value.UpdatedAt.Before(updated) {
		s.issues.Invalidate(key)
	}
	s.comments.InvalidateBefore(key, updated)
}

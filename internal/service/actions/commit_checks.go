package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// commitChecksAPI is what the read of every check of a commit needs of the
// GitHub client.
type commitChecksAPI interface {
	CommitChecksAll(ctx context.Context, repo core.RepoRef, sha string, notes bool) (core.CommitChecks, error)
}

// CommitChecksQuery selects every check of commit SHA of Repo: the check
// runs of GitHub Actions and of other Apps, and the commit statuses. Notes
// also reads the annotations of the check runs, which a view asks for only
// where it shows them.
type CommitChecksQuery struct {
	Repo  core.RepoRef
	SHA   string
	Notes bool
}

var errNoSHA = errors.New("commit checks query names no commit")

func (q CommitChecksQuery) normalize() CommitChecksQuery {
	q.SHA = strings.ToLower(q.SHA)
	return q
}

func commitChecksKey(q CommitChecksQuery) string {
	key := "commit-checks:" + repoID(q.Repo) + "@" + q.SHA
	if q.Notes {
		key += "+notes"
	}
	return key
}

// commitCache keeps the checks of commits by SHA in two caches: while any
// check is pending they stay fresh for a moment, as the checks of a commit
// in progress move, and once all completed, for long.
type commitCache struct {
	live, done *cache.Cache[core.CommitChecks]
}

func newCommitCache(live, done time.Duration, capacity cache.Option) commitCache {
	return commitCache{
		live: cache.New[core.CommitChecks](cache.WithTTL(live), capacity),
		done: cache.New[core.CommitChecks](cache.WithTTL(done), capacity),
	}
}

// get returns the checks kept under key: those that are fresh, else any.
func (c commitCache) get(key string) (cache.Entry[core.CommitChecks], cache.State) {
	d, ds := c.done.Get(key)
	if ds == cache.Fresh {
		return d, ds
	}
	l, ls := c.live.Get(key)
	if ls != cache.Miss {
		return l, ls
	}
	return d, ds
}

func (c commitCache) invalidateTag(tag string) {
	c.live.InvalidateTag(tag)
	c.done.InvalidateTag(tag)
}

// CachedCommitChecks returns the cached checks of q, fresh or stale,
// without a request. It reports false if they aren't in memory. Checks read
// with their annotations are cached apart from those read without.
func (s *Service) CachedCommitChecks(q CommitChecksQuery) (core.CommitChecks, bool) {
	e, st := s.commit.get(commitChecksKey(q.normalize()))
	return e.Value, st != cache.Miss
}

// FreshCommitChecks reports whether CommitChecks returns the checks of q
// without a request. It does no I/O.
func (s *Service) FreshCommitChecks(q CommitChecksQuery) bool {
	_, st := s.commit.get(commitChecksKey(q.normalize()))
	return st == cache.Fresh
}

// CommitChecks returns every check run and commit status of the commit q
// names, whatever its age: unlike Checks, it is not for the head of a pull
// request. Contexts past the first hundred are read too, in more queries.
// The entry is by SHA, fresh for the live TTL while any check is pending,
// and for the TTL of the rest once all completed. Nothing is kept beyond
// memory.
func (s *Service) CommitChecks(ctx context.Context, q CommitChecksQuery) (core.CommitChecks, error) {
	q = q.normalize()
	if q.SHA == "" {
		return core.CommitChecks{}, errNoSHA
	}
	key := commitChecksKey(q)
	if e, st := s.commit.get(key); st == cache.Fresh {
		return e.Value, nil
	}
	e, err := s.commit.live.Fetch(ctx, key, func(ctx context.Context, _ cache.Entry[core.CommitChecks], _ bool) (cache.Entry[core.CommitChecks], error) {
		c, err := s.api.CommitChecksAll(ctx, q.Repo, q.SHA, q.Notes)
		if err != nil {
			return cache.Entry[core.CommitChecks]{}, err
		}
		e := cache.Entry[core.CommitChecks]{Value: c, Tags: []string{repoTag(q.Repo)}}
		if !c.Pending() {
			s.commit.done.Set(key, e)
		}
		return e, nil
	})
	if err != nil {
		return core.CommitChecks{}, fmt.Errorf("checks of %s@%s: %w", q.Repo, q.SHA, err)
	}
	return e.Value, nil
}
